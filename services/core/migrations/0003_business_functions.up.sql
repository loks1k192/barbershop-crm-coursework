-- 0003. Серверные функции: вычисления и составные бизнес-операции.
--
-- Составные операции объявлены SECURITY DEFINER: прикладная роль crm_app не имеет
-- прямых прав на запись в payments / bonus_transactions / loyalty_accounts /
-- appointment_services и может изменять их только через эти функции.
-- Функция выполняется в транзакции вызывающего: любое исключение откатывает
-- все изменения операции целиком.

-- Ставка начисления бонусов по уровню.
CREATE FUNCTION crm.accrual_rate(p_level crm.loyalty_level) RETURNS numeric
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE p_level WHEN 'gold' THEN 0.07 WHEN 'silver' THEN 0.05 ELSE 0.03 END::numeric
$$;

-- Стоимость записи по зафиксированным ценам.
CREATE FUNCTION crm.appointment_total(p_appointment_id bigint) RETURNS numeric
LANGUAGE sql STABLE AS $$
    SELECT coalesce(sum(price), 0)
    FROM crm.appointment_services
    WHERE appointment_id = p_appointment_id
$$;

-- ── Свободные слоты мастера ─────────────────────────────────────────────────
-- Сетка с шагом slot_step_min внутри смены; слот свободен, если не пересекается
-- ни с одной активной записью (NOT EXISTS). В «компактном» режиме предлагаются
-- только слоты, примыкающие к началу/концу смены или к соседним записям, чтобы
-- не дробить день мастера «окнами».
CREATE FUNCTION crm.free_slots(
    p_barber_id    integer,
    p_date         date,
    p_duration_min integer,
    p_respect_lead boolean     DEFAULT true,
    p_now          timestamptz DEFAULT now()
) RETURNS TABLE (slot_start timestamptz, slot_end timestamptz)
LANGUAGE sql STABLE AS $$
    WITH shift AS (
        SELECT crm.shift_start_ts(ws.work_date, ws.starts_at) AS sh_start,
               crm.shift_start_ts(ws.work_date, ws.ends_at)   AS sh_end,
               bs.slot_step_min, bs.compact_slots, bs.min_lead_min
        FROM crm.work_shifts ws
        JOIN crm.barber_settings bs ON bs.barber_id = ws.barber_id
        WHERE ws.barber_id = p_barber_id AND ws.work_date = p_date
    ), grid AS (
        SELECT g AS slot_start,
               g + make_interval(mins => p_duration_min) AS slot_end,
               shift.*
        FROM shift,
             generate_series(shift.sh_start,
                             shift.sh_end - make_interval(mins => p_duration_min),
                             make_interval(mins => shift.slot_step_min)) AS g
    )
    SELECT grid.slot_start, grid.slot_end
    FROM grid
    WHERE (NOT p_respect_lead OR grid.slot_start >= p_now + make_interval(mins => grid.min_lead_min))
      AND NOT EXISTS (
            SELECT 1 FROM crm.appointments a
            WHERE a.barber_id = p_barber_id
              AND a.status NOT IN ('cancelled', 'no_show')
              AND a.starts_at < grid.slot_end
              AND a.ends_at   > grid.slot_start)
      AND (NOT grid.compact_slots
           OR grid.slot_start = grid.sh_start
           OR grid.slot_end   = grid.sh_end
           OR EXISTS (
                SELECT 1 FROM crm.appointments a
                WHERE a.barber_id = p_barber_id
                  AND a.status NOT IN ('cancelled', 'no_show')
                  AND (a.ends_at = grid.slot_start OR a.starts_at = grid.slot_end)))
    ORDER BY grid.slot_start
$$;

-- ── Бизнес-операция 1: запись клиента ───────────────────────────────────────
-- Атомарно: поиск/создание клиента (+ бонусный счёт триггером), создание записи,
-- фиксация состава услуг и цен. Пересечения отсекаются EXCLUDE-ограничениями
-- (SQLSTATE 23P01), выход за смену — триггером (CR003).
CREATE FUNCTION crm.book_appointment(
    p_client_phone text,
    p_client_name  text,
    p_barber_id    integer,
    p_starts_at    timestamptz,
    p_service_ids  integer[],
    p_source       crm.booking_source DEFAULT 'online',
    p_comment      text               DEFAULT NULL
) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path = crm, pg_temp AS $$
DECLARE
    v_barber    crm.barbers;
    v_settings  crm.barber_settings;
    v_cnt       integer;
    v_duration  integer;
    v_client_id bigint;
    v_appt_id   bigint;
BEGIN
    IF p_service_ids IS NULL OR cardinality(p_service_ids) = 0 THEN
        RAISE EXCEPTION USING ERRCODE = 'CR007', MESSAGE = 'Не выбрано ни одной услуги';
    END IF;
    IF (SELECT count(DISTINCT s) FROM unnest(p_service_ids) s) <> cardinality(p_service_ids) THEN
        RAISE EXCEPTION USING ERRCODE = 'CR007', MESSAGE = 'Услуги в записи не должны повторяться';
    END IF;

    SELECT * INTO v_barber FROM crm.barbers WHERE barber_id = p_barber_id;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'CR001', MESSAGE = format('Мастер %s не найден', p_barber_id);
    END IF;
    IF v_barber.fired_at IS NOT NULL THEN
        RAISE EXCEPTION USING ERRCODE = 'CR007', MESSAGE = format('Мастер %s не работает', v_barber.full_name);
    END IF;

    -- BR-10: только активные услуги из прайса мастера
    SELECT count(*), sum(bsv.duration_min)
    INTO v_cnt, v_duration
    FROM crm.barber_services bsv
    JOIN crm.services s ON s.service_id = bsv.service_id
    WHERE bsv.barber_id = p_barber_id
      AND bsv.service_id = ANY (p_service_ids)
      AND bsv.is_active AND s.is_active;
    IF v_cnt <> cardinality(p_service_ids) THEN
        RAISE EXCEPTION USING ERRCODE = 'CR007',
            MESSAGE = 'Мастер не оказывает одну или несколько выбранных услуг';
    END IF;

    -- BR-11: окно онлайн-записи
    SELECT * INTO v_settings FROM crm.barber_settings WHERE barber_id = p_barber_id;
    IF p_source = 'online' THEN
        IF p_starts_at < now() + make_interval(mins => v_settings.min_lead_min) THEN
            RAISE EXCEPTION USING ERRCODE = 'CR008',
                MESSAGE = format('Онлайн-запись возможна не позднее чем за %s мин. до начала',
                                 v_settings.min_lead_min);
        END IF;
        IF p_starts_at > now() + make_interval(days => v_settings.max_days_ahead) THEN
            RAISE EXCEPTION USING ERRCODE = 'CR008',
                MESSAGE = format('Онлайн-запись открыта только на %s дн. вперёд', v_settings.max_days_ahead);
        END IF;
    ELSIF p_starts_at < now() - interval '15 minutes' THEN
        RAISE EXCEPTION USING ERRCODE = 'CR008', MESSAGE = 'Нельзя записать на прошедшее время';
    END IF;

    -- клиент идентифицируется по телефону
    INSERT INTO crm.clients (phone, full_name)
    VALUES (p_client_phone, btrim(p_client_name))
    ON CONFLICT (phone) DO NOTHING
    RETURNING client_id INTO v_client_id;
    IF v_client_id IS NULL THEN
        SELECT client_id INTO v_client_id FROM crm.clients WHERE phone = p_client_phone;
    END IF;

    INSERT INTO crm.appointments (client_id, barber_id, starts_at, ends_at, source, comment)
    VALUES (v_client_id, p_barber_id, p_starts_at,
            p_starts_at + make_interval(mins => v_duration), p_source, p_comment)
    RETURNING appointment_id INTO v_appt_id;

    INSERT INTO crm.appointment_services (appointment_id, service_id, price, duration_min)
    SELECT v_appt_id, bsv.service_id, bsv.price, bsv.duration_min
    FROM crm.barber_services bsv
    WHERE bsv.barber_id = p_barber_id AND bsv.service_id = ANY (p_service_ids);

    RETURN v_appt_id;
END $$;

-- ── Бизнес-операция 2: завершение визита с оплатой и бонусами ───────────────
-- Атомарно изменяет 5 таблиц: appointments (статус), appointment_status_history
-- (триггер), payments, bonus_transactions, loyalty_accounts.
-- Бонусный счёт блокируется SELECT ... FOR UPDATE — это исключает потерянное
-- обновление баланса при параллельном завершении двух визитов одного клиента.
CREATE FUNCTION crm.complete_appointment(
    p_appointment_id  bigint,
    p_method          crm.payment_method,
    p_bonus_to_redeem numeric DEFAULT 0
) RETURNS TABLE (
    total_amount    numeric,
    bonus_redeemed  numeric,
    paid_money      numeric,
    bonus_accrued   numeric,
    bonus_balance   numeric,
    loyalty_level   crm.loyalty_level
)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = crm, pg_temp AS $$
DECLARE
    v_appt    crm.appointments;
    v_acc     crm.loyalty_accounts;
    v_total   numeric;
    v_prepaid numeric;
    v_due     numeric;
    v_money   numeric;
    v_accrual numeric;
    v_redeem  numeric := coalesce(p_bonus_to_redeem, 0);
BEGIN
    IF p_method = 'bonus' THEN
        RAISE EXCEPTION USING ERRCODE = '22023',
            MESSAGE = 'Оплата бонусами задаётся параметром p_bonus_to_redeem';
    END IF;
    IF v_redeem < 0 THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Сумма списания бонусов не может быть отрицательной';
    END IF;

    SELECT * INTO v_appt FROM crm.appointments WHERE appointment_id = p_appointment_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'CR001', MESSAGE = format('Запись %s не найдена', p_appointment_id);
    END IF;
    IF v_appt.status NOT IN ('scheduled', 'confirmed', 'in_progress') THEN
        RAISE EXCEPTION USING ERRCODE = 'CR002',
            MESSAGE = format('Запись %s уже в статусе «%s»', p_appointment_id, v_appt.status);
    END IF;

    v_total := crm.appointment_total(p_appointment_id);
    SELECT coalesce(sum(amount), 0) INTO v_prepaid FROM crm.payments WHERE appointment_id = p_appointment_id;
    v_due := v_total - v_prepaid;

    SELECT * INTO v_acc FROM crm.loyalty_accounts WHERE client_id = v_appt.client_id FOR UPDATE;
    IF v_redeem > v_acc.bonus_balance THEN
        RAISE EXCEPTION USING ERRCODE = 'CR009',
            MESSAGE = format('Недостаточно бонусов: запрошено %s, доступно %s', v_redeem, v_acc.bonus_balance);
    END IF;
    IF v_redeem > round(v_total * 0.30, 2) OR v_redeem > v_due THEN
        RAISE EXCEPTION USING ERRCODE = 'CR009',
            MESSAGE = format('Бонусами можно оплатить не более 30%% чека (%s ₽)', least(round(v_total * 0.30, 2), v_due));
    END IF;

    UPDATE crm.appointments SET status = 'completed' WHERE appointment_id = p_appointment_id;

    v_money := v_due - v_redeem;
    IF v_redeem > 0 THEN
        INSERT INTO crm.payments (appointment_id, amount, method) VALUES (p_appointment_id, v_redeem, 'bonus');
        INSERT INTO crm.bonus_transactions (client_id, appointment_id, kind, amount, comment)
        VALUES (v_appt.client_id, p_appointment_id, 'redeem', -v_redeem, 'Оплата визита бонусами');
    END IF;
    IF v_money > 0 THEN
        INSERT INTO crm.payments (appointment_id, amount, method) VALUES (p_appointment_id, v_money, p_method);
    END IF;

    v_accrual := round((v_total - v_redeem) * crm.accrual_rate(v_acc.level), 0);
    IF v_accrual > 0 THEN
        INSERT INTO crm.bonus_transactions (client_id, appointment_id, kind, amount, comment)
        VALUES (v_appt.client_id, p_appointment_id, 'accrual', v_accrual,
                format('Начисление %s%% за визит', round(crm.accrual_rate(v_acc.level) * 100)));
    END IF;

    UPDATE crm.loyalty_accounts la
    SET total_spent = la.total_spent + (v_total - v_redeem)
    WHERE la.client_id = v_appt.client_id
    RETURNING la.bonus_balance, la.level INTO v_acc.bonus_balance, v_acc.level;

    RETURN QUERY SELECT v_total, v_redeem, v_money, coalesce(v_accrual, 0), v_acc.bonus_balance, v_acc.level;
END $$;

-- ── Бизнес-операция 3: отмена записи ────────────────────────────────────────
CREATE FUNCTION crm.cancel_appointment(p_appointment_id bigint, p_reason text)
RETURNS crm.appointments
LANGUAGE plpgsql AS $$
DECLARE
    v_appt crm.appointments;
BEGIN
    IF p_reason IS NULL OR btrim(p_reason) = '' THEN
        RAISE EXCEPTION USING ERRCODE = '22023', MESSAGE = 'Укажите причину отмены';
    END IF;
    UPDATE crm.appointments
    SET status = 'cancelled', cancel_reason = btrim(p_reason)
    WHERE appointment_id = p_appointment_id
    RETURNING * INTO v_appt;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'CR001', MESSAGE = format('Запись %s не найдена', p_appointment_id);
    END IF;
    RETURN v_appt;
END $$;

-- ── Перенос записи на другое время (длительность сохраняется) ───────────────
CREATE FUNCTION crm.reschedule_appointment(p_appointment_id bigint, p_new_start timestamptz)
RETURNS crm.appointments
LANGUAGE plpgsql SECURITY DEFINER SET search_path = crm, pg_temp AS $$
DECLARE
    v_appt crm.appointments;
BEGIN
    UPDATE crm.appointments
    SET starts_at = p_new_start,
        ends_at   = p_new_start + (ends_at - starts_at)
    WHERE appointment_id = p_appointment_id
    RETURNING * INTO v_appt;
    IF NOT FOUND THEN
        RAISE EXCEPTION USING ERRCODE = 'CR001', MESSAGE = format('Запись %s не найдена', p_appointment_id);
    END IF;
    IF v_appt.status NOT IN ('scheduled', 'confirmed') THEN
        RAISE EXCEPTION USING ERRCODE = 'CR002',
            MESSAGE = format('Перенести можно только запланированную запись (статус «%s»)', v_appt.status);
    END IF;
    RETURN v_appt;
END $$;
