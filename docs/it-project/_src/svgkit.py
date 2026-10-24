"""Мини-библиотека для генерации SVG/PNG-диаграмм (Чен, IDEF0, BPMN).

Текст измеряется реальным шрифтом (Liberation Sans) через PIL, поэтому подписи
не вылезают за фигуры. PNG рендерится из SVG через cairosvg.
"""
from __future__ import annotations

import math
import os
from html import escape

from PIL import ImageFont

FONT_FAMILY = "Liberation Sans, Arial, Helvetica, sans-serif"
_FONT_FILES = {
    (False, False): "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
    (True, False): "/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf",
    (False, True): "/usr/share/fonts/truetype/liberation/LiberationSans-Italic.ttf",
    (True, True): "/usr/share/fonts/truetype/liberation/LiberationSans-BoldItalic.ttf",
}
_font_cache: dict = {}


def text_width(s: str, size: float = 14, bold: bool = False, italic: bool = False) -> float:
    key = (bold, italic, round(size * 4))
    f = _font_cache.get(key)
    if f is None:
        path = _FONT_FILES[(bold, italic)]
        f = ImageFont.truetype(path, size=int(round(size * 4))) if os.path.exists(path) else None
        _font_cache[key] = f
    if f is None:
        return len(s) * size * 0.55
    return f.getlength(s) / 4.0


def wrap(text: str, max_width: float, size: float = 14, bold: bool = False) -> list[str]:
    """Перенос по словам; явные '\n' сохраняются."""
    lines: list[str] = []
    for para in text.split("\n"):
        words = para.split(" ")
        cur = ""
        for w in words:
            cand = (cur + " " + w).strip()
            if cur and text_width(cand, size, bold) > max_width:
                lines.append(cur)
                cur = w
            else:
                cur = cand
        lines.append(cur)
    return lines


class Canvas:
    def __init__(self, width: float, height: float, background: str = "#ffffff"):
        self.w = width
        self.h = height
        self.bg = background
        self.layers: dict[int, list[str]] = {}
        self.defs: list[str] = []
        self._markers: set[str] = set()

    # ── низкоуровневое ──────────────────────────────────────────────────────
    def add(self, el: str, z: int = 0) -> None:
        self.layers.setdefault(z, []).append(el)

    @staticmethod
    def _style(stroke="#222", width=1.6, fill="none", dash=None, extra="") -> str:
        s = f'stroke="{stroke}" stroke-width="{width}" fill="{fill}"'
        if dash:
            s += f' stroke-dasharray="{dash}"'
        return s + (" " + extra if extra else "")

    # ── маркеры стрелок ─────────────────────────────────────────────────────
    def marker(self, name: str) -> str:
        if name in self._markers:
            return name
        self._markers.add(name)
        if name == "arrow":  # залитая стрелка
            self.defs.append(
                '<marker id="arrow" viewBox="0 0 12 12" refX="11" refY="6" markerWidth="11" '
                'markerHeight="11" orient="auto-start-reverse" markerUnits="userSpaceOnUse">'
                '<path d="M0,1 L12,6 L0,11 z" fill="#222"/></marker>')
        elif name == "arrow-open":  # открытая стрелка (message flow BPMN)
            self.defs.append(
                '<marker id="arrow-open" viewBox="0 0 12 12" refX="11" refY="6" markerWidth="12" '
                'markerHeight="12" orient="auto" markerUnits="userSpaceOnUse">'
                '<path d="M1,1 L11,6 L1,11 z" fill="#fff" stroke="#222" stroke-width="1.3"/></marker>')
        elif name == "circle-open":  # начало message flow
            self.defs.append(
                '<marker id="circle-open" viewBox="0 0 10 10" refX="5" refY="5" markerWidth="9" '
                'markerHeight="9" orient="auto" markerUnits="userSpaceOnUse">'
                '<circle cx="5" cy="5" r="4" fill="#fff" stroke="#222" stroke-width="1.3"/></marker>')
        elif name == "assoc":  # ассоциация данных — открытая «галочка»
            self.defs.append(
                '<marker id="assoc" viewBox="0 0 12 12" refX="11" refY="6" markerWidth="11" '
                'markerHeight="11" orient="auto" markerUnits="userSpaceOnUse">'
                '<path d="M1,1 L11,6 L1,11" fill="none" stroke="#555" stroke-width="1.3"/></marker>')
        elif name == "slash":  # default flow
            self.defs.append(
                '<marker id="slash" viewBox="0 0 12 12" refX="6" refY="6" markerWidth="12" '
                'markerHeight="12" orient="auto" markerUnits="userSpaceOnUse">'
                '<path d="M3,11 L9,1" stroke="#222" stroke-width="1.5"/></marker>')
        return name

    # ── примитивы ───────────────────────────────────────────────────────────
    def rect(self, x, y, w, h, rx=0, z=1, **st):
        self.add(f'<rect x="{x:.1f}" y="{y:.1f}" width="{w:.1f}" height="{h:.1f}" rx="{rx}" '
                 f'{self._style(**st)}/>', z)

    def ellipse(self, cx, cy, rx, ry, z=1, **st):
        self.add(f'<ellipse cx="{cx:.1f}" cy="{cy:.1f}" rx="{rx:.1f}" ry="{ry:.1f}" {self._style(**st)}/>', z)

    def circle(self, cx, cy, r, z=1, **st):
        self.add(f'<circle cx="{cx:.1f}" cy="{cy:.1f}" r="{r:.1f}" {self._style(**st)}/>', z)

    def polygon(self, pts, z=1, **st):
        p = " ".join(f"{x:.1f},{y:.1f}" for x, y in pts)
        self.add(f'<polygon points="{p}" {self._style(**st)}/>', z)

    def path(self, d, z=1, **st):
        self.add(f'<path d="{d}" {self._style(**st)}/>', z)

    def polyline(self, pts, z=0, start=None, end=None, mid=None, **st):
        p = " ".join(f"{x:.1f},{y:.1f}" for x, y in pts)
        m = ""
        if start:
            m += f' marker-start="url(#{self.marker(start)})"'
        if end:
            m += f' marker-end="url(#{self.marker(end)})"'
        self.add(f'<polyline points="{p}" {self._style(**st)}{m} stroke-linejoin="round"/>', z)

    def line(self, x1, y1, x2, y2, z=0, **st):
        self.polyline([(x1, y1), (x2, y2)], z=z, **st)

    def text(self, x, y, s, size=14, bold=False, italic=False, anchor="middle", color="#111",
             underline=False, dash_underline=False, z=5, baseline_shift=0.35):
        weight = ' font-weight="bold"' if bold else ""
        style = ' font-style="italic"' if italic else ""
        yy = y + size * baseline_shift
        self.add(f'<text x="{x:.1f}" y="{yy:.1f}" font-family="{FONT_FAMILY}" font-size="{size}" '
                 f'text-anchor="{anchor}" fill="{color}"{weight}{style}>{escape(s)}</text>', z)
        if underline or dash_underline:
            w = text_width(s, size, bold, italic)
            x0 = x - w / 2 if anchor == "middle" else (x - w if anchor == "end" else x)
            uy = yy + size * 0.18
            self.line(x0, uy, x0 + w, uy, z=z, stroke=color, width=1.1,
                      dash="4,3" if dash_underline else None)

    def mtext(self, x, y, lines, size=14, bold=False, italic=False, anchor="middle", color="#111",
              line_h=1.2, z=5):
        """Многострочный текст, центрированный по вертикали относительно y."""
        n = len(lines)
        top = y - (n - 1) * size * line_h / 2
        for i, ln in enumerate(lines):
            self.text(x, top + i * size * line_h, ln, size, bold, italic, anchor, color, z=z)

    # ── вывод ───────────────────────────────────────────────────────────────
    def svg(self) -> str:
        body = []
        for z in sorted(self.layers):
            body.extend(self.layers[z])
        defs = "<defs>" + "".join(self.defs) + "</defs>" if self.defs else ""
        return (f'<svg xmlns="http://www.w3.org/2000/svg" width="{self.w:.0f}" height="{self.h:.0f}" '
                f'viewBox="0 0 {self.w:.0f} {self.h:.0f}">{defs}'
                f'<rect width="100%" height="100%" fill="{self.bg}"/>' + "\n".join(body) + "</svg>")

    def save(self, base_path: str, png_scale: float = 1.5) -> None:
        svg = self.svg()
        with open(base_path + ".svg", "w", encoding="utf-8") as f:
            f.write(svg)
        try:
            import cairosvg
            cairosvg.svg2png(bytestring=svg.encode("utf-8"), write_to=base_path + ".png", scale=png_scale)
        except ImportError:  # PNG — опционально
            print("cairosvg не установлен: PNG не создан")


def clip_to_rect(cx, cy, w, h, tx, ty):
    """Точка на границе прямоугольника (центр cx,cy) в направлении (tx,ty)."""
    dx, dy = tx - cx, ty - cy
    if dx == 0 and dy == 0:
        return cx, cy
    sx = (w / 2) / abs(dx) if dx else math.inf
    sy = (h / 2) / abs(dy) if dy else math.inf
    s = min(sx, sy)
    return cx + dx * s, cy + dy * s
