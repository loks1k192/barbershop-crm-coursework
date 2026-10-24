"""ER-диаграмма предметной области «Сеть барбершопов» в нотации П. Чена.

Запуск:  python3 docs/it-project/_src/er_chen.py
Результат: docs/it-project/01-er-chen/er-chen-full.{svg,png}
           docs/it-project/01-er-chen/er-chen-overview.{svg,png}

Нотация:
  прямоугольник            — сильная сущность
  двойной прямоугольник    — слабая сущность
  ромб                     — связь;   двойной ромб — идентифицирующая связь
  овал                     — атрибут; подчёркнут — ключ; пунктирное подчёркивание —
                             частичный ключ слабой сущности
  пунктирный овал          — производный (вычисляемый) атрибут
  овал с дочерними овалами — составной атрибут
  одинарная линия          — частичное участие; двойная линия — полное участие
  1 / N / M                — кардинальность (мощность) связи
"""
import math
import os
import sys

sys.path.insert(0, os.path.dirname(__file__))
from svgkit import Canvas, text_width, clip_to_rect  # noqa: E402

OUT = os.path.join(os.path.dirname(__file__), "..", "01-er-chen")

FS_ENT = 17      # шрифт сущности
FS_REL = 14      # шрифт связи
FS_ATTR = 13     # шрифт атрибута
ENT_H = 58
REL_H = 74
ATTR_H = 36

# ── Сущности ──────────────────────────────────────────────────────────────────
# attrs: (подпись, вид, угол°, расстояние, [дочерние для composite])
# угол: 0 — вправо, 90 — вниз, -90 — вверх, 180 — влево
E = {
    "branch": dict(label="ФИЛИАЛ", x=1150, y=330, weak=False, attrs=[
        ("Код филиала", "key", -90, 165),
        ("Название", "plain", -140, 215),
        ("Адрес", "plain", -40, 215),
        ("Телефон", "plain", 0, 260),
        ("Часы работы", "composite", 180, 270, [("Открытие", -125, 125), ("Закрытие", 125, 125)]),
        ("Активен", "plain", 35, 235),
    ]),
    "barber": dict(label="МАСТЕР", x=1150, y=1080, weak=False, attrs=[
        ("Код мастера", "key", -58, 215),
        ("ФИО", "plain", -25, 240),
        ("Телефон", "plain", 25, 235),
        ("Логин", "plain", 78, 175),
        ("Хеш пароля", "plain", 103, 215),
        ("Грейд", "plain", 130, 200),
        ("Дата приёма", "plain", 180, 265),
        ("Дата увольнения", "plain", -133, 270),
        ("Стаж", "derived", -108, 190),
    ]),
    "shift": dict(label="СМЕНА", x=330, y=760, weak=True, attrs=[
        ("Дата смены", "pkey", -110, 175),
        ("Время начала", "plain", 180, 230),
        ("Время окончания", "plain", 132, 205),
        ("Длительность", "derived", -150, 215),
    ]),
    "settings": dict(label="НАСТРОЙКИ ЗАПИСИ", x=330, y=1450, weak=True, attrs=[
        ("Шаг сетки слотов", "plain", -150, 230),
        ("Компактные слоты", "plain", 180, 260),
        ("Мин. время до записи", "plain", 135, 215),
        ("Горизонт записи, дней", "plain", 80, 165),
    ]),
    "category": dict(label="КАТЕГОРИЯ УСЛУГ", x=2350, y=330, weak=False, attrs=[
        ("Код категории", "key", -90, 165),
        ("Название", "plain", -15, 270),
        ("Описание", "plain", 30, 255),
    ]),
    "service": dict(label="УСЛУГА", x=2350, y=1080, weak=False, attrs=[
        ("Код услуги", "key", -45, 225),
        ("Название", "plain", -10, 270),
        ("Описание", "plain", 22, 255),
        ("Базовая цена", "plain", 52, 265),
        ("Базовая длительность", "plain", 100, 225),
        ("Активна", "plain", 150, 210),
    ]),
    "appointment": dict(label="ЗАПИСЬ (ВИЗИТ)", x=1750, y=1900, weak=False, attrs=[
        ("Код записи", "key", -90, 175),
        ("Начало", "plain", -117, 300),
        ("Окончание", "plain", -63, 300),
        ("Статус", "plain", 180, 270),
        ("Источник", "plain", 203, 300),
        ("Комментарий", "plain", 120, 225),
        ("Причина отмены", "plain", 140, 330),
        ("Стоимость", "derived", 70, 235),
    ]),
    "client": dict(label="КЛИЕНТ", x=650, y=2350, weak=False, attrs=[
        ("Код клиента", "key", -90, 175),
        ("Телефон", "plain", -125, 250),
        ("ФИО", "composite", -158, 290, [("Фамилия", -115, 100), ("Имя", -190, 95)]),
        ("E-mail", "plain", 180, 300),
        ("Дата рождения", "plain", 155, 270),
        ("Возраст", "derived", 132, 330),
        ("Заметки", "plain", 105, 240),
        ("Согласие на рассылку", "plain", -55, 290),
        ("Число визитов", "derived", 20, 300),
        ("Последний визит", "derived", 50, 260),
    ]),
    "loyalty": dict(label="БОНУСНЫЙ СЧЁТ", x=650, y=3000, weak=True, attrs=[
        ("Уровень", "plain", 180, 270),
        ("Баланс бонусов", "plain", 135, 210),
        ("Сумма покупок", "plain", 90, 155),
    ]),
    "bonus_tx": dict(label="БОНУСНАЯ ОПЕРАЦИЯ", x=1750, y=3000, weak=False, attrs=[
        ("Код операции", "key", -40, 235),
        ("Вид операции", "plain", 15, 255),
        ("Сумма", "plain", 70, 165),
        ("Дата и время", "plain", 120, 175),
    ]),
    "payment": dict(label="ОПЛАТА", x=2950, y=1900, weak=False, attrs=[
        ("Код оплаты", "key", -90, 165),
        ("Сумма", "plain", -35, 195),
        ("Способ оплаты", "plain", 35, 200),
        ("Дата и время", "plain", 90, 160),
    ]),
    "review": dict(label="ОТЗЫВ", x=2950, y=2480, weak=True, attrs=[
        ("Оценка (1–5)", "plain", -35, 200),
        ("Текст отзыва", "plain", 30, 205),
        ("Дата отзыва", "plain", 90, 150),
    ]),
    "history": dict(label="ИЗМЕНЕНИЕ СТАТУСА", x=2700, y=3000, weak=True, attrs=[
        ("Момент изменения", "pkey", 90, 150),
        ("Старый статус", "plain", 20, 255),
        ("Новый статус", "plain", -25, 245),
        ("Кем изменён", "plain", 140, 175),
    ]),
}

# ── Связи ─────────────────────────────────────────────────────────────────────
# ends: (сущность, кардинальность, полное_участие)
R = {
    "works_in": dict(label="РАБОТАЕТ В", x=1150, y=700, ident=False,
                     ends=[("branch", "1", False), ("barber", "N", True)]),
    "held_in": dict(label="ПРОВОДИТСЯ В", x=740, y=545, ident=False,
                    ends=[("branch", "1", False), ("shift", "N", True)]),
    "works_shift": dict(label="ВЫХОДИТ НА", x=740, y=920, ident=True,
                        ends=[("barber", "1", False), ("shift", "N", True)]),
    "has_settings": dict(label="ИМЕЕТ", x=740, y=1265, ident=True,
                         ends=[("barber", "1", True), ("settings", "1", True)]),
    "in_category": dict(label="ОТНОСИТСЯ К", x=2350, y=700, ident=False,
                        ends=[("category", "1", False), ("service", "N", True)]),
    "provides": dict(label="ОКАЗЫВАЕТ", x=1750, y=1080, ident=False,
                     ends=[("barber", "M", False), ("service", "N", False)],
                     attrs=[("Цена мастера", "plain", -145, 175), ("Длительность", "plain", -90, 125),
                            ("Активна", "plain", -35, 175)]),
    "serves": dict(label="ОБСЛУЖИВАЕТ", x=1450, y=1490, ident=False,
                   ends=[("barber", "1", False), ("appointment", "N", True)]),
    "contains": dict(label="ВКЛЮЧАЕТ", x=2050, y=1490, ident=False,
                     ends=[("appointment", "M", True), ("service", "N", False)],
                     attrs=[("Цена на момент записи", "plain", 0, 245), ("Длительность", "plain", 38, 200)]),
    "books": dict(label="ОФОРМЛЯЕТ", x=1200, y=2125, ident=False,
                  ends=[("client", "1", False), ("appointment", "N", True)]),
    "owns": dict(label="ВЛАДЕЕТ", x=650, y=2690, ident=True,
                 ends=[("client", "1", True), ("loyalty", "1", True)]),
    "moves": dict(label="ДВИЖЕНИЕ", x=1200, y=3000, ident=False,
                  ends=[("loyalty", "1", False), ("bonus_tx", "N", True)]),
    "basis": dict(label="ОСНОВАНИЕ", x=1750, y=2450, ident=False,
                  ends=[("appointment", "1", False), ("bonus_tx", "N", False)]),
    "paid_by": dict(label="ОПЛАЧИВАЕТСЯ", x=2350, y=1900, ident=False,
                    ends=[("appointment", "1", False), ("payment", "N", True)]),
    "rated": dict(label="ОЦЕНИВАЕТСЯ", x=2350, y=2190, ident=True,
                  ends=[("appointment", "1", False), ("review", "1", True)]),
    "tracks": dict(label="ФИКСИРУЕТ", x=2225, y=2450, ident=True,
                   ends=[("appointment", "1", False), ("history", "N", True)]),
}


def ent_size(e):
    return text_width(e["label"], FS_ENT, bold=True) + 46, ENT_H


def rel_size(r):
    return text_width(r["label"], FS_REL, bold=True) * 1.35 + 60, REL_H


def attr_size(label):
    return text_width(label, FS_ATTR) + 34, ATTR_H


def polar(x, y, ang, d):
    a = math.radians(ang)
    return x + d * math.cos(a), y + d * math.sin(a)


def clip_to_diamond(cx, cy, w, h, tx, ty):
    dx, dy = tx - cx, ty - cy
    if dx == 0 and dy == 0:
        return cx, cy
    s = 1 / (abs(dx) / (w / 2) + abs(dy) / (h / 2))
    return cx + dx * s, cy + dy * s


def draw_attr(c, px, py, label, kind, ang, dist, children=None):
    ax, ay = polar(px, py, ang, dist)
    w, h = attr_size(label)
    c.line(px, py, ax, ay, z=0, stroke="#333", width=1.2)
    dash = "6,4" if kind == "derived" else None
    c.ellipse(ax, ay, w / 2, h / 2, z=2, stroke="#333", width=1.4, fill="#fffdf2", dash=dash)
    c.text(ax, ay, label, FS_ATTR, underline=(kind == "key"), dash_underline=(kind == "pkey"))
    for ch in children or []:
        draw_attr(c, ax, ay, ch[0], "plain", ch[1], ch[2])


def draw_double_line(c, p1, p2, gap=3.2):
    (x1, y1), (x2, y2) = p1, p2
    L = math.hypot(x2 - x1, y2 - y1) or 1
    nx, ny = -(y2 - y1) / L * gap, (x2 - x1) / L * gap
    c.line(x1 + nx, y1 + ny, x2 + nx, y2 + ny, z=0, stroke="#111", width=1.5)
    c.line(x1 - nx, y1 - ny, x2 - nx, y2 - ny, z=0, stroke="#111", width=1.5)


def render(with_attrs: bool, path: str):
    W, H = 3420, 3260
    c = Canvas(W, H)
    title = "ER-диаграмма предметной области «Сеть барбершопов» (нотация П. Чена)"
    if not with_attrs:
        title += " — обзорная схема"
    c.text(W / 2, 46, title, 26, bold=True)

    # связи и линии участия
    for r in R.values():
        rw, rh = rel_size(r)
        for ent_key, card, total in r["ends"]:
            e = E[ent_key]
            ew, eh = ent_size(e)
            p_ent = clip_to_rect(e["x"], e["y"], ew + (10 if e["weak"] else 0), eh + (10 if e["weak"] else 0),
                                 r["x"], r["y"])
            p_rel = clip_to_diamond(r["x"], r["y"], rw, rh, e["x"], e["y"])
            if total:
                draw_double_line(c, p_rel, p_ent)
            else:
                c.line(*p_rel, *p_ent, z=0, stroke="#111", width=1.5)
            # кардинальность — у сущности, со смещением от линии
            dx, dy = p_rel[0] - p_ent[0], p_rel[1] - p_ent[1]
            L = math.hypot(dx, dy) or 1
            bx, by = p_ent[0] + dx / L * 34, p_ent[1] + dy / L * 34
            ox, oy = -dy / L * 15, dx / L * 15
            c.circle(bx + ox, by + oy, 12, z=3, stroke="none", width=0, fill="#ffffff")
            c.text(bx + ox, by + oy, card, 17, bold=True, color="#b0161b", z=6)
        pts = [(r["x"], r["y"] - rh / 2), (r["x"] + rw / 2, r["y"]), (r["x"], r["y"] + rh / 2),
               (r["x"] - rw / 2, r["y"])]
        c.polygon(pts, z=2, stroke="#1d3d7a", width=1.8, fill="#e8f0ff")
        if r["ident"]:
            k = 0.80
            inner = [(r["x"], r["y"] - rh / 2 * k), (r["x"] + rw / 2 * k, r["y"]),
                     (r["x"], r["y"] + rh / 2 * k), (r["x"] - rw / 2 * k, r["y"])]
            c.polygon(inner, z=2, stroke="#1d3d7a", width=1.4, fill="none")
        c.text(r["x"], r["y"], r["label"], FS_REL, bold=True, color="#1d3d7a")
        if with_attrs:
            for a in r.get("attrs", []):
                draw_attr(c, r["x"], r["y"], a[0], a[1], a[2], a[3])

    # сущности
    for e in E.values():
        ew, eh = ent_size(e)
        if with_attrs:
            for a in e["attrs"]:
                draw_attr(c, e["x"], e["y"], a[0], a[1], a[2], a[3], a[4] if len(a) > 4 else None)
        elif any(a[1] in ("key", "pkey") for a in e["attrs"]):
            for a in e["attrs"]:
                if a[1] in ("key", "pkey"):
                    draw_attr(c, e["x"], e["y"], a[0], a[1], a[2], a[3])
        if e["weak"]:
            c.rect(e["x"] - ew / 2 - 6, e["y"] - eh / 2 - 6, ew + 12, eh + 12, z=2, stroke="#0d4d2b", width=1.6,
                   fill="#e6f6ea")
            c.rect(e["x"] - ew / 2, e["y"] - eh / 2, ew, eh, z=2, stroke="#0d4d2b", width=1.6, fill="#e6f6ea")
        else:
            c.rect(e["x"] - ew / 2, e["y"] - eh / 2, ew, eh, z=2, stroke="#0d4d2b", width=2, fill="#e6f6ea")
        c.text(e["x"], e["y"], e["label"], FS_ENT, bold=True, color="#0d4d2b")

    draw_legend(c, 2760, 70)
    c.save(path)


def draw_legend(c, x, y):
    w, h = 610, 600
    c.rect(x, y, w, h, rx=8, z=1, stroke="#555", width=1.2, fill="#fafafa")
    c.text(x + w / 2, y + 26, "Условные обозначения", 17, bold=True)
    rows = [
        ("ent", "Сильная сущность"),
        ("weak", "Слабая сущность"),
        ("rel", "Связь"),
        ("irel", "Идентифицирующая связь"),
        ("attr", "Атрибут"),
        ("key", "Ключевой атрибут"),
        ("pkey", "Частичный ключ слабой сущности"),
        ("der", "Производный (вычисляемый) атрибут"),
        ("part", "Частичное участие в связи"),
        ("total", "Полное (обязательное) участие"),
        ("card", "Кардинальность связи (1:1, 1:N, M:N)"),
    ]
    yy = y + 70
    cx = x + 95
    for kind, label in rows:
        if kind in ("ent", "weak"):
            if kind == "weak":
                c.rect(cx - 62, yy - 21, 124, 42, z=2, stroke="#0d4d2b", width=1.4, fill="#e6f6ea")
            c.rect(cx - 56, yy - 15, 112, 30, z=2, stroke="#0d4d2b", width=1.6, fill="#e6f6ea")
            c.text(cx, yy, "СУЩНОСТЬ", 12, bold=True, color="#0d4d2b")
        elif kind in ("rel", "irel"):
            pts = [(cx, yy - 21), (cx + 62, yy), (cx, yy + 21), (cx - 62, yy)]
            c.polygon(pts, z=2, stroke="#1d3d7a", width=1.6, fill="#e8f0ff")
            if kind == "irel":
                c.polygon([(cx, yy - 16), (cx + 48, yy), (cx, yy + 16), (cx - 48, yy)], z=2, stroke="#1d3d7a",
                          width=1.2)
            c.text(cx, yy, "СВЯЗЬ", 11, bold=True, color="#1d3d7a")
        elif kind in ("attr", "key", "pkey", "der"):
            c.ellipse(cx, yy, 62, 17, z=2, stroke="#333", width=1.3, fill="#fffdf2",
                      dash="6,4" if kind == "der" else None)
            c.text(cx, yy, "атрибут", 12, underline=(kind == "key"), dash_underline=(kind == "pkey"))
        elif kind == "part":
            c.line(cx - 60, yy, cx + 60, yy, z=2, stroke="#111", width=1.5)
        elif kind == "total":
            c.line(cx - 60, yy - 3, cx + 60, yy - 3, z=2, stroke="#111", width=1.5)
            c.line(cx - 60, yy + 3, cx + 60, yy + 3, z=2, stroke="#111", width=1.5)
        elif kind == "card":
            c.text(cx, yy, "1    N    M", 16, bold=True, color="#b0161b")
        c.text(x + 190, yy, label, 14, anchor="start")
        yy += 48


if __name__ == "__main__":
    os.makedirs(OUT, exist_ok=True)
    render(True, os.path.join(OUT, "er-chen-full"))
    render(False, os.path.join(OUT, "er-chen-overview"))
    print("ok")
