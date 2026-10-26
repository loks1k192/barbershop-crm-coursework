DROP TRIGGER IF EXISTS trg_loyalty_level ON crm.loyalty_accounts;
DROP TRIGGER IF EXISTS trg_bonus_tx_apply ON crm.bonus_transactions;
DROP TRIGGER IF EXISTS trg_reviews_validate ON crm.reviews;
DROP TRIGGER IF EXISTS trg_payments_validate ON crm.payments;
DROP TRIGGER IF EXISTS trg_appointments_status_history ON crm.appointments;
DROP TRIGGER IF EXISTS trg_appointments_validate ON crm.appointments;
DROP TRIGGER IF EXISTS trg_work_shifts_validate ON crm.work_shifts;
DROP TRIGGER IF EXISTS trg_clients_create_loyalty ON crm.clients;
DROP TRIGGER IF EXISTS trg_barbers_create_settings ON crm.barbers;
DROP TRIGGER IF EXISTS trg_appointments_updated_at ON crm.appointments;
DROP TRIGGER IF EXISTS trg_clients_updated_at ON crm.clients;
DROP TRIGGER IF EXISTS trg_barber_settings_updated_at ON crm.barber_settings;
DROP TRIGGER IF EXISTS trg_barbers_updated_at ON crm.barbers;
DROP TRIGGER IF EXISTS trg_branches_updated_at ON crm.branches;
DROP FUNCTION IF EXISTS crm.tg_loyalty_level(), crm.loyalty_level_for(numeric), crm.tg_bonus_tx_apply(),
    crm.tg_reviews_validate(), crm.tg_payments_validate(), crm.tg_appointments_status_history(),
    crm.tg_appointments_validate(), crm.tg_work_shifts_validate(), crm.tg_clients_create_loyalty(),
    crm.tg_barbers_create_settings(), crm.tg_set_updated_at(), crm.shift_start_ts(date, time),
    crm.is_schema_owner(), crm.local_tz();
