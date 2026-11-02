"""Функциональная модель (IDEF0) CRM сети барбершопов.

Запуск:  python3 docs/it-project/_src/idef0.py
Результат: docs/it-project/02-functional-model/idef0-A-0.{svg,png}, idef0-A0..., idef0-A4..., idef0-A5...

Правила IDEF0, соблюдаемые генератором:
  * функциональный блок — прямоугольник с глагольным названием и номером (A1…);
  * Input входит слева, Control — сверху, Output выходит справа, Mechanism — снизу;
  * граничные стрелки декомпозиции помечены ICOM-кодами (I1, C2, O3, M1) родительского блока;
  * блоки декомпозиции расположены «ступенькой» (доминирование сверху-слева вниз-вправо),
    от 3 до 6 блоков на диаграмме;
  * обратные связи по входу проходят под блоками, по управлению — над блоками;
  * разветвление стрелки отмечено точкой.
"""
import os
import sys

sys.path.insert(0, os.path.dirname(__file__))
from svgkit import Canvas, text_width, wrap  # noqa: E402

OUT = os.path.join(os.path.dirname(__file__), "..", "02-functional-model")
FS = 13          # подписи стрелок
BOX_FS = 15      # названия блоков
INK = "#1b1b1b"
BOX_FILL = "#eef4ff"


class Diagram:
    def __init__(self, w, h, node, title, number):
        self.c = Canvas(w, h)
        self.w, self.h = w, h
        self.node, self.title, self.number = node, title, number
        self.boxes = {}
        self.top, self.bottom = 70, h - 70           # рабочая область внутри рамки
        self.left, self.right = 40, w - 40

    # ── блок ────────────────────────────────────────────────────────────────
    def box(self, key, x, y, w, h, title, num):
        self.boxes[key] = (x, y, w, h)
        c = self.c
        c.rect(x, y, w, h, z=3, stroke=INK, width=2, fill=BOX_FILL)
        lines = wrap(title, w - 24, BOX_FS, bold=True)
        c.mtext(x + w / 2, y + h / 2 - 6, lines, BOX_FS, bold=True, z=6)
        c.text(x + w - 8, y + h - 12, num, 13, bold=True, anchor="end", z=6)
        return key

    def L(self, key, dy):  # точка на левой стороне
        x, y, w, h = self.boxes[key]
        return (x, y + dy)

    def R(self, key, dy):
        x, y, w, h = self.boxes[key]
        return (x + w, y + dy)

    def T(self, key, dx):
        x, y, w, h = self.boxes[key]
        return (x + dx, y)

    def B(self, key, dx):
        x, y, w, h = self.boxes[key]
        return (x + dx, y + h)

    # ── стрелка ─────────────────────────────────────────────────────────────
    def arrow(self, pts, label=None, seg=0, where=0.5, side=-1, code=None, code_at="start",
              head=True, dashed=False, maxw=260, label_dx=0):
        """pts — ломаная; label ставится у сегмента seg (над/под ним, side -1/+1)."""
        c = self.c
        c.polyline(pts, z=1, stroke=INK, width=1.6, end="arrow" if head else None,
                   dash="7,4" if dashed else None)
        if label:
            (x1, y1), (x2, y2) = pts[seg], pts[seg + 1]
            lines = wrap(label, maxw, FS)
            if y1 == y2:   # горизонтальный сегмент — подпись над/под
                mx = x1 + (x2 - x1) * where + label_dx
                n = len(lines)
                base = y1 - 8 - (n - 1) * FS * 1.15 if side < 0 else y1 + 14
                for i, ln in enumerate(lines):
                    self._label(mx, base + i * FS * 1.15, ln)
            else:          # вертикальный сегмент — подпись сбоку
                my = y1 + (y2 - y1) * where
                n = len(lines)
                top = my - (n - 1) * FS * 1.15 / 2
                anchor = "end" if side < 0 else "start"
                for i, ln in enumerate(lines):
                    self._label(x1 + (-7 if side < 0 else 7), top + i * FS * 1.15, ln, anchor)
        if code:
            x, y = pts[0] if code_at == "start" else pts[-1]
            (ax, ay), (bx, by) = (pts[0], pts[1]) if code_at == "start" else (pts[-2], pts[-1])
            if ay == by:   # горизонтальная граница
                dx = -6 if code_at == "start" and bx > ax else 6
                if code_at == "end":
                    dx = 6
                c.text(x + (-4 if code_at == "start" else 4), y - 9, code, 12, bold=True,
                       anchor="end" if code_at == "start" else "start", color="#b0161b", z=7)
            else:
                c.text(x + 6, y + (14 if code_at == "start" and by > ay else -6), code, 12, bold=True,
                       anchor="start", color="#b0161b", z=7)

    def _label(self, x, y, s, anchor="middle"):
        w = text_width(s, FS)
        x0 = x - w / 2 if anchor == "middle" else (x - w if anchor == "end" else x)
        self.c.rect(x0 - 2, y - FS * 0.75, w + 4, FS * 1.1, z=4, stroke="none", width=0, fill="#ffffff")
        self.c.text(x, y, s, FS, anchor=anchor, z=5)

    def dot(self, x, y):
        self.c.circle(x, y, 3.6, z=6, stroke=INK, width=1, fill=INK)

    def tunnel(self, x, y, vertical=True):
        """Туннельная стрелка: скобки «( )» у конца."""
        if vertical:
            self.c.path(f"M{x - 7},{y - 9} Q{x - 12},{y} {x - 7},{y + 9}", z=6, stroke=INK, width=1.4)
            self.c.path(f"M{x + 7},{y - 9} Q{x + 12},{y} {x + 7},{y + 9}", z=6, stroke=INK, width=1.4)

    # ── рамка IDEF0 ─────────────────────────────────────────────────────────
    def frame(self, author="Команда проекта Barber CRM", project="CRM сети барбершопов", date="03.11.2026"):
        c, w, h = self.c, self.w, self.h
        c.rect(10, 10, w - 20, h - 20, z=0, stroke=INK, width=1.5, fill="none")
        # шапка
        c.line(10, 50, w - 10, 50, z=0, stroke=INK, width=1)
        cols = [(10, f"ИСПОЛЬЗУЕТСЯ: курсовой проект"), (w * 0.22, f"АВТОР: {author}"),
                (w * 0.47, f"ПРОЕКТ: {project}"), (w * 0.70, f"ДАТА: {date}"),
                (w * 0.82, "СТАТУС: РАБОЧАЯ ВЕРСИЯ")]
        for i, (x, s) in enumerate(cols):
            if i:
                c.line(x, 10, x, 50, z=0, stroke=INK, width=1)
            c.text(x + 10, 30, s, 13, anchor="start")
        # подвал
        c.line(10, h - 50, w - 10, h - 50, z=0, stroke=INK, width=1)
        c.line(w * 0.12, h - 50, w * 0.12, h - 10, z=0, stroke=INK, width=1)
        c.line(w * 0.86, h - 50, w * 0.86, h - 10, z=0, stroke=INK, width=1)
        c.text(20, h - 30, f"УЗЕЛ: {self.node}", 15, bold=True, anchor="start")
        c.text(w * 0.49, h - 30, f"НАЗВАНИЕ: {self.title}", 17, bold=True)
        c.text(w * 0.87, h - 30, f"НОМЕР: {self.number}", 15, bold=True, anchor="start")

    def save(self, name):
        self.frame()
        self.c.save(os.path.join(OUT, name))


# ════════════════════════════════════════════════════════════════════════════
# A-0 — контекстная диаграмма
# ════════════════════════════════════════════════════════════════════════════
def diagram_a_minus_0():
    d = Diagram(2000, 1250, "A-0", "Управлять деятельностью сети барбершопов", "FM-1")
    bx, by, bw, bh = 720, 420, 560, 330
    d.box("A0", bx, by, bw, bh, "Управлять деятельностью сети барбершопов", "A0")

    ins = [("Запросы клиентов на запись (онлайн, по телефону, визит)", 70),
           ("Сведения о филиалах, мастерах и услугах", 135),
           ("Клиентская база из прежних систем (CSV)", 200),
           ("Оплата клиента", 265)]
    for i, (lbl, dy) in enumerate(ins, 1):
        d.arrow([(60, by + dy), (bx, by + dy)], lbl, side=-1, where=0.45, maxw=560)
    ctrls = [("Регламенты сети: правила записи, отмены и оплаты", 80),
             ("Программа лояльности", 210),
             ("Законодательство: 152-ФЗ, 54-ФЗ", 340),
             ("Прайс-листы и часы работы филиалов", 470)]
    for i, (lbl, dx) in enumerate(ctrls):
        d.arrow([(bx + dx, 90), (bx + dx, by)], None)
        d._label(bx + dx + 8, 130 + i * 60, lbl, anchor="start")
    outs = [("Подтверждения и напоминания о записи", 40),
            ("Оказанные услуги", 90),
            ("Чеки и платёжные документы", 140),
            ("Бонусные операции, уровень лояльности", 190),
            ("Аналитические отчёты и KPI", 240),
            ("Журнал аудита изменений", 290)]
    for lbl, dy in outs:
        d.arrow([(bx + bw, by + dy), (1940, by + dy)], lbl, side=-1, where=0.52, maxw=560)
    mechs = [("Администраторы филиалов", 90), ("Барберы", 210), ("Управляющий сетью", 330),
             ("CRM-система Barber CRM (REST API, PostgreSQL)", 470)]
    for i, (lbl, dx) in enumerate(mechs):
        d.arrow([(bx + dx, 1170), (bx + dx, by + bh)], None)
        d._label(bx + dx + 8, by + bh + 60 + i * 55, lbl, anchor="start")

    c = d.c
    c.text(80, 1000, "Цель моделирования: определить функции CRM-системы сети барбершопов и",
           15, anchor="start")
    c.text(80, 1023, "информационные потоки между ними для проектирования БД и приложения.", 15, anchor="start")
    c.text(80, 1055, "Точка зрения: управляющий сетью барбершопов.", 15, anchor="start", bold=True)
    d.save("idef0-A-0")


# ════════════════════════════════════════════════════════════════════════════
# A0 — декомпозиция
# ════════════════════════════════════════════════════════════════════════════
def diagram_a0():
    W, H = 2700, 1780
    d = Diagram(W, H, "A0", "Управлять деятельностью сети барбершопов", "FM-2")
    bw, bh = 290, 150
    xs = [250, 625, 1000, 1375, 1750, 2125]
    ys = [300, 490, 680, 870, 1060, 1250]
    names = [("A1", "Вести справочники: филиалы, услуги, мастера, прайсы"),
             ("A2", "Планировать график работы мастеров"),
             ("A3", "Управлять клиентской базой и лояльностью"),
             ("A4", "Записывать клиентов на услуги"),
             ("A5", "Обслуживать клиентов и проводить расчёт"),
             ("A6", "Формировать аналитику и отчётность")]
    for (k, t), x, y in zip(names, xs, ys):
        d.box(k, x, y, bw, bh, t, k)
    LB, RB, TB, BB = 50, W - 50, 85, H - 80
    A = d.arrow

    # ── внешние входы ───────────────────────────────────────────────────────
    A([(LB, ys[0] + 75), d.L("A1", 75)], "Сведения о филиалах, мастерах и услугах", where=0.5, code="I2", maxw=200)
    A([(LB, ys[2] + 40), d.L("A3", 40)], "Клиентская база из прежних систем (CSV)", where=0.25, code="I3", maxw=230)
    A([(LB, ys[3] + 35), d.L("A4", 35)], "Запросы клиентов на запись", where=0.18, code="I1", maxw=230)
    A([(LB, ys[4] + 55), d.L("A5", 55)], "Оплата клиента", where=0.12, code="I4")

    # ── внешнее управление ──────────────────────────────────────────────────
    cx1 = xs[3] + 60
    A([(cx1, TB), d.T("A4", 60)], None, code="C1")
    d._label(cx1 + 130, TB + 32, "Регламенты сети: правила записи, отмены, оплаты")
    A([(cx1, TB + 60), (xs[4] + 60, TB + 60), d.T("A5", 60)], None)
    d.dot(cx1, TB + 60)
    cx2 = xs[2] + 70
    A([(cx2, TB), d.T("A3", 70)], None, code="C2")
    d._label(cx2 - 75, TB + 32, "Программа лояльности")
    A([(cx2, TB + 95), (xs[4] + 110, TB + 95), d.T("A5", 110)], None)
    d.dot(cx2, TB + 95)
    cx3 = xs[2] + 200
    A([(cx3, TB), d.T("A3", 200)], None, code="C3")
    d._label(cx3 + 8, TB + 160, "152-ФЗ «О персональных данных»", anchor="start")
    cx4 = xs[5] + 140
    A([(cx4, TB), d.T("A6", 140)], None, code="C4")
    d._label(cx4, TB + 32, "Требования к отчётности")

    # ── A1 → ────────────────────────────────────────────────────────────────
    y1 = ys[0] + 40
    A([d.R("A1", 40), (xs[3] + 140, y1), d.T("A4", 140)], "Каталог услуг и прайсы мастеров", where=0.30)
    A([(xs[3] + 140, y1), (xs[4] + 160, y1), d.T("A5", 160)], None)
    d.dot(xs[3] + 140, y1)
    A([d.R("A1", 115), (xs[0] + bw + 30, ys[0] + 115), (xs[0] + bw + 30, ys[1] + 75), d.L("A2", 75)],
      "Мастера и филиалы", seg=1, where=0.5, side=-1, maxw=120)

    # ── A2 → ────────────────────────────────────────────────────────────────
    A([d.R("A2", 40), (xs[3] + 200, ys[1] + 40), d.T("A4", 200)], "График смен", where=0.25)
    A([d.R("A2", 110), (xs[1] + bw + 18, ys[1] + 110), (xs[1] + bw + 18, ys[5] + 115), d.L("A6", 115)],
      "Фактический график (часы смен)", seg=2, where=0.22, side=-1, maxw=260)

    # ── A3 → ────────────────────────────────────────────────────────────────
    A([d.R("A3", 40), (RB, ys[2] + 40)], "Отчёт о загрузке клиентской базы", where=0.82, code="O6", code_at="end")
    xa = xs[2] + bw + 35
    A([d.R("A3", 110), (xa, ys[2] + 110), (xa, ys[3] + 95), d.L("A4", 95)], "Карточки клиентов", seg=1, where=0.55, side=-1, maxw=110)
    A([(xa, ys[3] + 95), (xa, ys[4] + 120), d.L("A5", 120)], "Бонусные счета", seg=1, where=0.62)
    d.dot(xa, ys[3] + 95)
    A([d.R("A3", 135), (xs[5] - 70, ys[2] + 135), (xs[5] - 70, ys[5] + 60), d.L("A6", 60)],
      "Клиентская база", seg=0, where=0.62)

    # ── A4 → ────────────────────────────────────────────────────────────────
    A([d.R("A4", 30), (RB, ys[3] + 30)], "Подтверждения и напоминания о записи", where=0.7, code="O1", code_at="end")
    xb = xs[3] + bw + 30
    A([d.R("A4", 85), (xb, ys[3] + 85), (xb, ys[4] + 30), d.L("A5", 30)], "Записи на обслуживание", seg=1, where=0.62, side=1, maxw=110)
    # обратная связь: новые клиенты → A3 (под блоком A4)
    yfb = ys[3] + bh + 20
    xfb = xs[2] - 45
    A([d.R("A4", 128), (xs[3] + bw + 12, ys[3] + 128), (xs[3] + bw + 12, yfb), (xfb, yfb), (xfb, ys[2] + 75), d.L("A3", 75)],
      "Новые клиенты", seg=2, where=0.3, side=1)

    # ── A5 → ────────────────────────────────────────────────────────────────
    A([d.R("A5", 25), (RB, ys[4] + 25)], "Оказанные услуги", where=0.72, code="O2", code_at="end")
    A([d.R("A5", 55), (RB, ys[4] + 55)], "Чеки и платёжные документы", where=0.72, code="O3", code_at="end")
    # обратная связь: бонусные операции → A3
    yfb2 = ys[4] + bh + 18
    xfb2 = xs[2] - 22
    A([d.R("A5", 92), (xs[4] + bw + 14, ys[4] + 92), (xs[4] + bw + 14, yfb2), (xfb2, yfb2), (xfb2, ys[2] + 110), d.L("A3", 110)],
      "Бонусные операции, изменение уровня", seg=2, where=0.45, side=1, maxw=300)
    A([(xs[4] + bw + 14, yfb2), (RB, yfb2)], "Бонусные операции", where=0.8, code="O4", code_at="end")
    d.dot(xs[4] + bw + 14, yfb2)
    A([d.R("A5", 128), (xs[4] + bw + 40, ys[4] + 128), (xs[4] + bw + 40, ys[5] + 95), d.L("A6", 95)],
      "Визиты, оплаты, отзывы", seg=1, where=0.3, side=-1, maxw=110)

    # ── A6 → ────────────────────────────────────────────────────────────────
    A([d.R("A6", 45), (RB, ys[5] + 45)], "Аналитические отчёты и KPI", where=0.55, code="O5", code_at="end")
    A([d.R("A6", 85), (RB, ys[5] + 85)], "Журнал аудита", where=0.55, code="O7", code_at="end")
    # обратная связь по управлению: решения → A1, A2 (над блоками)
    yt = ys[0] - 75
    A([d.R("A6", 125), (RB - 120, ys[5] + 125), (RB - 120, yt + 30), (xs[1] + 180, yt + 30), d.T("A2", 180)],
      "Управленческие решения: график, прайсы, штат", seg=2, where=0.9, side=-1, maxw=330)
    A([(xs[1] + 180, yt + 30), (xs[0] + 150, yt + 30), d.T("A1", 150)], None)
    d.dot(xs[1] + 180, yt + 30)

    # ── механизмы (шины снизу) ──────────────────────────────────────────────
    mech = [("Администратор филиала", "M1", [0, 1, 2, 3], 55),
            ("Барбер", "M2", [4], 115),
            ("Управляющий сетью", "M3", [0, 5], 175),
            ("CRM-система Barber CRM", "M4", [0, 1, 2, 3, 4, 5], 235)]
    for k, (lbl, code, boxes, dx) in enumerate(mech):
        ybus = BB - 120 + k * 28
        xin = 120 + k * 290
        xs_box = [xs[i] + dx for i in boxes]
        A([(xin, BB), (xin, ybus)], None, head=False, code=code, code_at="start")
        lo, hi = min(xs_box + [xin]), max(xs_box + [xin])
        d.c.polyline([(lo, ybus), (hi, ybus)], z=1, stroke=INK, width=1.6)
        d._label(xin + 8 + text_width(lbl, FS) / 2, BB - 22, lbl)
        for i, bxk in zip(boxes, xs_box):
            A([(bxk, ybus), d.B(names[i][0], dx)], None)
            if lo < bxk < hi:
                d.dot(bxk, ybus)
        if lo < xin < hi:
            d.dot(xin, ybus)
    d.save("idef0-A0")


# ════════════════════════════════════════════════════════════════════════════
# A4 — Записывать клиентов на услуги
# ════════════════════════════════════════════════════════════════════════════
def diagram_a4():
    W, H = 2300, 1450
    d = Diagram(W, H, "A4", "Записывать клиентов на услуги", "FM-3")
    bw, bh = 300, 150
    xs, ys = [300, 760, 1220, 1680], [270, 520, 770, 1020]
    names = [("A41", "Подобрать свободное время (crm.free_slots)"),
             ("A42", "Идентифицировать или зарегистрировать клиента"),
             ("A43", "Зарегистрировать запись (crm.book_appointment)"),
             ("A44", "Сопровождать запись: подтвердить, перенести, отменить")]
    for (k, t), x, y in zip(names, xs, ys):
        d.box(k, x, y, bw, bh, t, k)
    LB, RB, TB, BB = 50, W - 50, 85, H - 80
    A = d.arrow

    # входы
    A([(LB, ys[0] + 60), d.L("A41", 60)], "Запрос: мастер, услуги, желаемая дата", where=0.45, code="I1", maxw=230)
    A([(LB + 150, ys[0] + 60), (LB + 150, ys[1] + 50), d.L("A42", 50)], "Телефон и имя клиента", seg=1, where=0.6)
    d.dot(LB + 150, ys[0] + 60)
    A([(LB + 150, ys[1] + 50), (LB + 150, ys[3] + 40), d.L("A44", 40)], "Запрос на перенос или отмену", seg=1, where=0.62, maxw=300)
    d.dot(LB + 150, ys[1] + 50)
    A([(LB, ys[1] + 110), d.L("A42", 110)], "Карточки клиентов", where=0.3, code="I2")

    # управление
    A([(xs[0] + 60, TB), d.T("A41", 60)], None, code="C3")
    d._label(xs[0] + 60, TB + 40, "График смен")
    A([(xs[0] + 180, TB), d.T("A41", 180)], None, code="C2")
    d._label(xs[0] + 220, TB + 85, "Прайсы мастеров (длительность)")
    A([(xs[0] + 180, TB + 130), (xs[2] + 230, TB + 130), d.T("A43", 230)], "Цены на момент записи", seg=1, where=0.25, side=1)
    d.dot(xs[0] + 180, TB + 130)
    A([(xs[0] + 255, TB), d.T("A41", 255)], None, code="C1")
    d._label(xs[0] + 420, TB + 30, "Регламенты: шаг сетки, окно онлайн-записи, правила отмены")
    A([(xs[0] + 255, TB + 60), (xs[2] + 90, TB + 60), d.T("A43", 90)], None)
    d.dot(xs[0] + 255, TB + 60)
    A([(xs[2] + 90, TB + 60), (xs[3] + 150, TB + 60), d.T("A44", 150)], None)
    d.dot(xs[2] + 90, TB + 60)

    # потоки между блоками
    A([d.R("A41", 60), (xs[2] - 60, ys[0] + 60), (xs[2] - 60, ys[2] + 45), d.L("A43", 45)],
      "Выбранный свободный слот", seg=0, where=0.5)
    A([d.R("A42", 60), (xs[2] - 30, ys[1] + 60), (xs[2] - 30, ys[2] + 105), d.L("A43", 105)],
      "Карточка клиента", seg=0, where=0.42)
    A([d.R("A42", 120), (RB, ys[1] + 120)], "Новые клиенты", where=0.8, code="O3", code_at="end")
    A([d.R("A43", 60), (xs[3] - 40, ys[2] + 60), (xs[3] - 40, ys[3] + 95), d.L("A44", 95)],
      "Запись (scheduled)", seg=1, where=0.3, side=1, maxw=150)
    A([d.R("A43", 115), (RB, ys[2] + 115)], "Уведомление об отказе: время занято или вне смены", where=0.62, code="O1", code_at="end")
    A([d.R("A44", 40), (RB, ys[3] + 40)], "Подтверждения и напоминания", where=0.55, code="O1", code_at="end")
    A([d.R("A44", 85), (RB, ys[3] + 85)], "Записи на обслуживание", where=0.55, code="O2", code_at="end")
    # обратная связь: освободившееся время → A41 (под блоками)
    yfb = ys[3] + bh + 30
    A([d.R("A44", 130), (xs[3] + bw + 25, ys[3] + 130), (xs[3] + bw + 25, yfb), (xs[0] - 40, yfb),
       (xs[0] - 40, ys[0] + 120), d.L("A41", 120)], "Освободившееся время (отмена, перенос)", seg=2, where=0.5, side=1, maxw=360)

    # механизмы
    for k, (lbl, code, dx) in enumerate([("Администратор / клиент (самозапись)", "M1", 70),
                                          ("CRM-система Barber CRM", "M4", 220)]):
        ybus = BB - 70 + k * 30
        xin = 130 + k * 340
        A([(xin, BB), (xin, ybus)], None, head=False, code=code)
        d._label(xin + 8 + text_width(lbl, FS) / 2, BB - 18, lbl)
        lo, hi = min(xin, xs[0] + dx), xs[3] + dx
        d.c.polyline([(lo, ybus), (hi, ybus)], z=1, stroke=INK, width=1.6)
        if lo < xin:
            d.dot(xin, ybus)
        for i, x in enumerate(xs):
            A([(x + dx, ybus), d.B(names[i][0], dx)], None)
            if lo < x + dx < hi:
                d.dot(x + dx, ybus)
    d.save("idef0-A4")


# ════════════════════════════════════════════════════════════════════════════
# A5 — Обслуживать клиентов и проводить расчёт
# ════════════════════════════════════════════════════════════════════════════
def diagram_a5():
    W, H = 2500, 1600
    d = Diagram(W, H, "A5", "Обслуживать клиентов и проводить расчёт", "FM-4")
    bw, bh = 290, 150
    xs, ys = [300, 710, 1120, 1530, 1940], [260, 480, 700, 920, 1140]
    names = [("A51", "Принять клиента и начать визит"),
             ("A52", "Оказать услуги"),
             ("A53", "Рассчитать стоимость и принять оплату"),
             ("A54", "Начислить и списать бонусы, пересчитать уровень"),
             ("A55", "Получить отзыв о визите")]
    for (k, t), x, y in zip(names, xs, ys):
        d.box(k, x, y, bw, bh, t, k)
    LB, RB, TB, BB = 50, W - 50, 85, H - 80
    A = d.arrow

    A([(LB, ys[0] + 60), d.L("A51", 60)], "Записи на обслуживание", where=0.4, code="I1")
    A([(LB, ys[2] + 50), d.L("A53", 50)], "Оплата клиента (наличные, карта, онлайн)", where=0.35, code="I2", maxw=320)
    A([(LB, ys[2] + 110), d.L("A53", 110)], "Бонусный баланс клиента", where=0.35, code="I3")

    A([(xs[0] + 145, TB), d.T("A51", 145)], None, code="C1")
    d._label(xs[0] + 153, TB + 30, "Регламенты: начало не ранее 30 мин, неявка — после начала визита", anchor="start")
    A([(xs[0] + 145, TB + 60), (xs[2] + 80, TB + 60), d.T("A53", 80)], None)
    d.dot(xs[0] + 145, TB + 60)
    A([(xs[2] + 80, TB + 60), (xs[4] + 145, TB + 60), d.T("A55", 145)], None)
    d.dot(xs[2] + 80, TB + 60)
    d._label(xs[2] + 330, TB + 85, "Лимит списания бонусов 30 % чека; отзыв ≤ 30 дней")
    A([(xs[1] + 145, TB + 120), d.T("A52", 145)], None)
    A([(xs[1] - 200, TB + 120), (xs[1] + 145, TB + 120)], None, head=False)
    A([(xs[1] - 200, TB), (xs[1] - 200, TB + 120)], None, head=False, code="C3")
    d._label(xs[1] - 192, TB + 100, "Состав услуг и цены записи", anchor="start")
    A([(xs[1] + 145, TB + 120), (xs[2] + 210, TB + 120), d.T("A53", 210)], None)
    d.dot(xs[1] + 145, TB + 120)
    A([(xs[3] + 145, TB), d.T("A54", 145)], None, code="C2")
    d._label(xs[3] + 145, TB + 140, "Программа лояльности: ставки 3/5/7 %, пороги уровней")

    A([d.R("A51", 50), (xs[1] - 40, ys[0] + 50), (xs[1] - 40, ys[1] + 75), d.L("A52", 75)], "Визит начат (in_progress)", seg=1, where=0.3, side=1, maxw=150)
    A([d.R("A51", 115), (RB, ys[0] + 115)], "Неявка (no_show)", where=0.85, code="O4", code_at="end")
    A([d.R("A52", 50), (RB, ys[1] + 50)], "Оказанные услуги", where=0.8, code="O1", code_at="end")
    A([d.R("A52", 110), (xs[2] - 40, ys[1] + 110), (xs[2] - 40, ys[2] + 80), d.L("A53", 80)], "Услуги выполнены", seg=1, where=0.3, side=1, maxw=150)
    A([d.R("A53", 40), (RB, ys[2] + 40)], "Чеки и платёжные документы", where=0.75, code="O2", code_at="end")
    A([d.R("A53", 95), (xs[3] - 40, ys[2] + 95), (xs[3] - 40, ys[3] + 75), d.L("A54", 75)], "Оплаченный визит (completed)", seg=1, where=0.3, side=1, maxw=150)
    A([d.R("A54", 50), (RB, ys[3] + 50)], "Бонусные операции, уровень лояльности", where=0.7, code="O3", code_at="end")
    A([d.R("A54", 110), (xs[4] - 40, ys[3] + 110), (xs[4] - 40, ys[4] + 75), d.L("A55", 75)], "Закрытый визит", seg=1, where=0.3, side=1, maxw=150)
    A([d.R("A55", 75), (RB, ys[4] + 75)], "Визиты, оплаты, отзывы", where=0.5, code="O4", code_at="end")
    # обратная связь: при ошибке оплаты визит возвращается к расчёту (под блоками)
    yfb = ys[2] + bh + 25
    A([d.R("A53", 135), (xs[2] + bw + 18, ys[2] + 135), (xs[2] + bw + 18, yfb), (xs[2] - 70, yfb),
       (xs[2] - 70, ys[2] + 125), d.L("A53", 125)], "Отказ: превышен лимит бонусов — повторный расчёт", seg=2, where=0.45, side=1, maxw=380, dashed=False)

    for k, (lbl, code, boxes, dx) in enumerate([("Барбер", "M2", [0, 1], 60),
                                                 ("Администратор филиала", "M1", [0, 2, 4], 140),
                                                 ("CRM-система Barber CRM", "M4", [0, 1, 2, 3, 4], 230)]):
        ybus = BB - 95 + k * 28
        xin = (110, 210, 410)[k]
        xs_box = [xs[i] + dx for i in boxes]
        A([(xin, BB), (xin, ybus)], None, head=False, code=code)
        d._label(xin + 8 + text_width(lbl, FS) / 2, BB - 15 - k * 0, lbl)
        lo, hi = min(xs_box + [xin]), max(xs_box + [xin])
        d.c.polyline([(lo, ybus), (hi, ybus)], z=1, stroke=INK, width=1.6)
        if lo < xin < hi:
            d.dot(xin, ybus)
        for i, x in zip(boxes, xs_box):
            A([(x, ybus), d.B(names[i][0], dx)], None)
            if lo < x < hi:
                d.dot(x, ybus)
    d.save("idef0-A5")


if __name__ == "__main__":
    os.makedirs(OUT, exist_ok=True)
    diagram_a_minus_0()
    diagram_a0()
    diagram_a4()
    diagram_a5()
    print("ok")
