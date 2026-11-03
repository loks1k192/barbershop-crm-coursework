"""BPMN 2.0: модели бизнес-процессов CRM сети барбершопов.

Запуск:  python3 docs/it-project/_src/bpmn.py
Результат (docs/it-project/03-bpmn/):
  *.bpmn — BPMN 2.0 XML с диаграммной разметкой (BPMN DI), открывается в Camunda
           Modeler / demo.bpmn.io;
  *.svg, *.png — изображения, построенные из той же модели.

Используемые элементы нотации: пулы и дорожки; стартовые, промежуточные (ловящие/
бросающие) и конечные события — простые, сообщения, таймера, ошибки; граничное
событие ошибки; задачи — пользовательская, сервисная, отправки, получения, ручная;
шлюзы — исключающий (XOR), параллельный (AND), основанный на событиях; потоки
управления, потоки сообщений, ассоциации с хранилищем данных.
"""
import os
import sys
from html import escape
from xml.sax.saxutils import quoteattr

sys.path.insert(0, os.path.dirname(__file__))
from svgkit import Canvas, text_width, wrap  # noqa: E402

OUT = os.path.join(os.path.dirname(__file__), "..", "03-bpmn")
INK = "#222"
TASK_W, TASK_H = 130, 80
EV_R = 18
GW = 25  # половина диагонали шлюза


class Model:
    def __init__(self, key, title):
        self.key, self.title = key, title
        self.pools = []      # dict(id, name, x, y, w, h, lanes=[(id,name,y,h)], process, black_box)
        self.nodes = {}      # id -> dict(kind, name, cx, cy, pool, lane, ...)
        self.flows = []      # dict(id, kind, src, tgt, name, pts)
        self.stores = {}

    # ── структура ────────────────────────────────────────────────────────────
    def pool(self, pid, name, x, y, w, lanes, black_box=False, h=None):
        ly = y
        ls = []
        for lid, lname, lh in lanes:
            ls.append((lid, lname, ly, lh))
            ly += lh
        self.pools.append(dict(id=pid, name=name, x=x, y=y, w=w, h=h or (ly - y), lanes=ls,
                               process="Process_" + pid, black_box=black_box))

    def lane_of(self, cy):
        for p in self.pools:
            for lid, _, ly, lh in p["lanes"]:
                if ly <= cy <= ly + lh:
                    return p, lid
        raise ValueError(cy)

    def node(self, nid, kind, name, cx, cy, ev=None, attached=None):
        p, lid = self.lane_of(cy)
        self.nodes[nid] = dict(id=nid, kind=kind, name=name, cx=cx, cy=cy, ev=ev, pool=p["id"], lane=lid,
                               attached=attached)
        return nid

    def task(self, nid, name, cx, cy, kind="userTask"):
        return self.node(nid, kind, name, cx, cy)

    def event(self, nid, kind, name, cx, cy, ev=None, attached=None, label="below"):
        self.node(nid, kind, name, cx, cy, ev, attached)
        self.nodes[nid]["label"] = label
        return nid

    def gateway(self, nid, kind, name, cx, cy):
        return self.node(nid, kind, name, cx, cy)

    def store(self, sid, name, cx, cy):
        self.stores[sid] = dict(id=sid, name=name, cx=cx, cy=cy)

    # ── геометрия ────────────────────────────────────────────────────────────
    def bounds(self, nid):
        n = self.nodes.get(nid) or self.stores[nid]
        k = n.get("kind", "store")
        if k.endswith("Task") or k == "task":
            return n["cx"] - TASK_W / 2, n["cy"] - TASK_H / 2, TASK_W, TASK_H
        if k.endswith("Gateway"):
            return n["cx"] - GW, n["cy"] - GW, 2 * GW, 2 * GW
        if k == "store":
            return n["cx"] - 30, n["cy"] - 30, 60, 60
        return n["cx"] - EV_R, n["cy"] - EV_R, 2 * EV_R, 2 * EV_R

    def port(self, nid, side):
        x, y, w, h = self.bounds(nid)
        return {"L": (x, y + h / 2), "R": (x + w, y + h / 2), "T": (x + w / 2, y), "B": (x + w / 2, y + h)}[side]

    def flow(self, fid, src, tgt, name=None, via=None, s="R", t="L", kind="seq", label_at=0, default=False):
        p0, p1 = self.port(src, s), self.port(tgt, t)
        pts = [p0] + (via or []) + [p1]
        if not via and p0[1] != p1[1] and p0[0] != p1[0]:   # автоматический ортогональный маршрут
            if s in "LR" and t in "LR":
                mx = (p0[0] + p1[0]) / 2
                pts = [p0, (mx, p0[1]), (mx, p1[1]), p1]
            elif s in "TB" and t in "LR":
                pts = [p0, (p0[0], p1[1]), p1]
            elif s in "LR" and t in "TB":
                pts = [p0, (p1[0], p0[1]), p1]
            else:
                my = (p0[1] + p1[1]) / 2
                pts = [p0, (p0[0], my), (p1[0], my), p1]
        self.flows.append(dict(id=fid, kind=kind, src=src, tgt=tgt, name=name, pts=pts, label_at=label_at,
                               default=default))

    def msg(self, fid, src, tgt, name=None, s="B", t="T", via=None):
        self.flow(fid, src, tgt, name, via=via, s=s, t=t, kind="msg")

    def assoc(self, fid, src, store, s="B", t="T", via=None):
        self.flow(fid, src, store, None, via=via, s=s, t=t, kind="data")

    # ════════════════════════════════════════════════════════════════════════
    # SVG
    # ════════════════════════════════════════════════════════════════════════
    def render(self, W, H):
        c = Canvas(W, H)
        c.text(W / 2, 28, self.title, 22, bold=True)
        for p in self.pools:
            c.rect(p["x"], p["y"], p["w"], p["h"], z=0, stroke=INK, width=1.8, fill="#ffffff")
            c.rect(p["x"], p["y"], 32, p["h"], z=0, stroke=INK, width=1.8, fill="#f4f4f4")
            self._vtext(c, p["x"] + 16, p["y"] + p["h"] / 2, p["name"], 15, bold=True)
            if not p["black_box"] and len(p["lanes"]) > 1:
                for lid, lname, ly, lh in p["lanes"]:
                    c.rect(p["x"] + 32, ly, p["w"] - 32, lh, z=0, stroke=INK, width=1.1, fill="none")
                    c.rect(p["x"] + 32, ly, 30, lh, z=0, stroke=INK, width=1.1, fill="#fafafa")
                    self._vtext(c, p["x"] + 47, ly + lh / 2, lname, 13)
        for f in self.flows:
            self._flow(c, f)
        for n in self.nodes.values():
            self._node(c, n)
        for s in self.stores.values():
            self._store(c, s)
        self._legend(c, W, H)
        return c

    @staticmethod
    def _vtext(c, x, y, s, size, bold=False):
        w = "bold" if bold else "normal"
        c.add(f'<text x="{x}" y="{y}" font-family="Liberation Sans, Arial" font-size="{size}" font-weight="{w}" '
              f'text-anchor="middle" dominant-baseline="middle" transform="rotate(-90 {x} {y})">{escape(s)}</text>', 2)

    def _flow(self, c, f):
        pts = f["pts"]
        if f["kind"] == "seq":
            c.polyline(pts, z=1, stroke=INK, width=1.6, end="arrow", start="slash" if f["default"] else None)
        elif f["kind"] == "msg":
            c.polyline(pts, z=1, stroke="#555", width=1.4, dash="8,5", start="circle-open", end="arrow-open")
        else:
            c.polyline(pts, z=1, stroke="#666", width=1.3, dash="2,4", end="assoc")
        if f["name"]:
            i = f["label_at"]
            (x1, y1), (x2, y2) = pts[i], pts[i + 1]
            lines = wrap(f["name"], 170, 12)
            if y1 == y2:
                x, y = (x1 + x2) / 2, y1 - 8 - (len(lines) - 1) * 14
                for k, ln in enumerate(lines):
                    self._lbl(c, x, y + k * 14, ln, "middle")
            else:
                x, y = x1 + 6, (y1 + y2) / 2 - (len(lines) - 1) * 7
                for k, ln in enumerate(lines):
                    self._lbl(c, x, y + k * 14, ln, "start")

    @staticmethod
    def _lbl(c, x, y, s, anchor):
        w = text_width(s, 12)
        x0 = x - w / 2 if anchor == "middle" else x
        c.rect(x0 - 2, y - 10, w + 4, 14, z=3, stroke="none", width=0, fill="#ffffff")
        c.text(x, y, s, 12, anchor=anchor, z=4, color="#333")

    def _node(self, c, n):
        k, cx, cy = n["kind"], n["cx"], n["cy"]
        if k.endswith("Task"):
            x, y = cx - TASK_W / 2, cy - TASK_H / 2
            c.rect(x, y, TASK_W, TASK_H, rx=10, z=5, stroke=INK, width=1.7, fill="#fffbea")
            self._task_icon(c, k, x + 8, y + 7)
            lines = wrap(n["name"], TASK_W - 14, 12.5)
            c.mtext(cx, cy + 6, lines, 12.5, z=6)
        elif k.endswith("Gateway"):
            c.polygon([(cx, cy - GW), (cx + GW, cy), (cx, cy + GW), (cx - GW, cy)], z=5, stroke=INK, width=1.7,
                      fill="#fff5f0")
            if k == "exclusiveGateway":
                c.line(cx - 9, cy - 9, cx + 9, cy + 9, z=6, stroke=INK, width=3)
                c.line(cx - 9, cy + 9, cx + 9, cy - 9, z=6, stroke=INK, width=3)
            elif k == "parallelGateway":
                c.line(cx, cy - 12, cx, cy + 12, z=6, stroke=INK, width=3)
                c.line(cx - 12, cy, cx + 12, cy, z=6, stroke=INK, width=3)
            elif k == "eventBasedGateway":
                c.circle(cx, cy, 13, z=6, stroke=INK, width=1.2)
                c.circle(cx, cy, 10, z=6, stroke=INK, width=1.2)
                pts = [(cx + 6 * __import__("math").cos(a), cy + 6 * __import__("math").sin(a))
                       for a in [(-90 + 72 * i) * 3.14159 / 180 for i in range(5)]]
                c.polygon(pts, z=6, stroke=INK, width=1.2)
            if n["name"]:
                for i, ln in enumerate(wrap(n["name"], 150, 12)):
                    self._lbl(c, cx, cy - GW - 10 - (len(wrap(n["name"], 150, 12)) - 1 - i) * 14, ln, "middle")
        else:
            thick = {"startEvent": 1.6, "endEvent": 4.2}.get(k, 1.6)
            fill = "#eaf7ea" if k == "startEvent" else ("#fbeaea" if k == "endEvent" else "#ffffff")
            c.circle(cx, cy, EV_R, z=5, stroke=INK, width=thick, fill=fill)
            if k in ("intermediateCatchEvent", "intermediateThrowEvent", "boundaryEvent"):
                c.circle(cx, cy, EV_R - 3.5, z=5, stroke=INK, width=1.3)
            throw = k in ("endEvent", "intermediateThrowEvent")
            self._ev_icon(c, n["ev"], cx, cy, throw)
            lines = wrap(n["name"] or "", 130, 12)
            if n.get("label") == "right":
                for i, ln in enumerate(lines):
                    self._lbl(c, cx + EV_R + 6, cy + 4 - (len(lines) - 1) * 7 + i * 14, ln, "start")
            else:
                for i, ln in enumerate(lines):
                    self._lbl(c, cx, cy + EV_R + 15 + i * 14, ln, "middle")

    @staticmethod
    def _task_icon(c, k, x, y):
        if k == "userTask":
            c.circle(x + 7, y + 5, 4, z=6, stroke=INK, width=1.1, fill="#fff")
            c.path(f"M{x},{y + 17} Q{x + 7},{y + 6} {x + 14},{y + 17} Z", z=6, stroke=INK, width=1.1, fill="#fff")
        elif k == "serviceTask":
            for dx, dy, r in ((7, 7, 6), (12, 12, 5)):
                c.circle(x + dx, y + dy, r, z=6, stroke=INK, width=1.1, fill="#fff")
                c.circle(x + dx, y + dy, r / 2.5, z=6, stroke=INK, width=1)
        elif k in ("sendTask", "receiveTask"):
            fill = INK if k == "sendTask" else "#fff"
            c.rect(x, y + 2, 17, 12, z=6, stroke=INK, width=1.1, fill=fill)
            c.polyline([(x, y + 2), (x + 8.5, y + 9), (x + 17, y + 2)], z=6,
                       stroke="#fff" if k == "sendTask" else INK, width=1.1)
        elif k == "manualTask":
            c.path(f"M{x},{y + 8} h10 a2,2 0 0 1 0,4 h-2 v3 h-8 z", z=6, stroke=INK, width=1.1, fill="#fff")

    @staticmethod
    def _ev_icon(c, ev, cx, cy, throw):
        if ev == "message":
            fill = INK if throw else "#fff"
            c.rect(cx - 8, cy - 6, 16, 12, z=6, stroke=INK, width=1.1, fill=fill)
            c.polyline([(cx - 8, cy - 6), (cx, cy + 1), (cx + 8, cy - 6)], z=6, stroke="#fff" if throw else INK,
                       width=1.1)
        elif ev == "timer":
            c.circle(cx, cy, 10, z=6, stroke=INK, width=1.3, fill="#fff")
            c.line(cx, cy, cx, cy - 7, z=6, stroke=INK, width=1.3)
            c.line(cx, cy, cx + 5, cy + 2, z=6, stroke=INK, width=1.3)
        elif ev == "error":
            c.polygon([(cx - 8, cy + 8), (cx - 3, cy - 7), (cx + 2, cy + 2), (cx + 8, cy - 8), (cx + 3, cy + 7),
                       (cx - 2, cy - 2)], z=6, stroke=INK, width=1.2, fill=INK if throw else "#fff")
        elif ev == "terminate":
            c.circle(cx, cy, 10, z=6, stroke=INK, width=1, fill=INK)

    def _store(self, c, s):
        cx, cy = s["cx"], s["cy"]
        x, y, w, h = cx - 30, cy - 30, 60, 60
        c.path(f"M{x},{y + 8} v{h - 16} a30,8 0 0 0 {w},0 v-{h - 16} a30,8 0 0 0 -{w},0 a30,8 0 0 0 {w},0",
               z=5, stroke=INK, width=1.5, fill="#eef4ff")
        for i, ln in enumerate(wrap(s["name"], 140, 12)):
            self._lbl(c, cx, cy + 46 + i * 14, ln, "middle")

    def _legend(self, c, W, H):
        pass

    # ════════════════════════════════════════════════════════════════════════
    # BPMN 2.0 XML
    # ════════════════════════════════════════════════════════════════════════
    def xml(self):
        out = ['<?xml version="1.0" encoding="UTF-8"?>',
               '<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" '
               'xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI" '
               'xmlns:dc="http://www.omg.org/spec/DD/20100524/DC" '
               'xmlns:di="http://www.omg.org/spec/DD/20100524/DI" '
               f'id="Definitions_{self.key}" targetNamespace="http://barber-crm.local/bpmn" '
               'exporter="barber-crm docs generator" exporterVersion="1.0">']
        out.append(f'  <bpmn:collaboration id="Collaboration_{self.key}">')
        for p in self.pools:
            pr = "" if p["black_box"] else f' processRef="{p["process"]}"'
            out.append(f'    <bpmn:participant id="{p["id"]}" name={quoteattr(p["name"])}{pr} />')
        for f in self.flows:
            if f["kind"] == "msg":
                nm = f' name={quoteattr(f["name"])}' if f["name"] else ""
                src = f["src"] if f["src"] in self.nodes else f["src"]
                out.append(f'    <bpmn:messageFlow id="{f["id"]}"{nm} sourceRef="{src}" targetRef="{f["tgt"]}" />')
        out.append('  </bpmn:collaboration>')

        for p in self.pools:
            if p["black_box"]:
                continue
            out.append(f'  <bpmn:process id="{p["process"]}" isExecutable="false">')
            if len(p["lanes"]) > 1:
                out.append(f'    <bpmn:laneSet id="LaneSet_{p["id"]}">')
                for lid, lname, _, _ in p["lanes"]:
                    out.append(f'      <bpmn:lane id="{lid}" name={quoteattr(lname)}>')
                    for n in self.nodes.values():
                        if n["lane"] == lid:
                            out.append(f'        <bpmn:flowNodeRef>{n["id"]}</bpmn:flowNodeRef>')
                    out.append('      </bpmn:lane>')
                out.append('    </bpmn:laneSet>')
            for n in self.nodes.values():
                if n["pool"] != p["id"]:
                    continue
                attrs = f'id="{n["id"]}" name={quoteattr(n["name"] or "")}'
                if n["kind"] == "boundaryEvent":
                    attrs += f' attachedToRef="{n["attached"]}"'
                defaults = [f["id"] for f in self.flows if f["kind"] == "seq" and f["src"] == n["id"] and f["default"]]
                if defaults:
                    attrs += f' default="{defaults[0]}"'
                out.append(f'    <bpmn:{n["kind"]} {attrs}>')
                for f in self.flows:
                    if f["kind"] == "seq" and f["tgt"] == n["id"]:
                        out.append(f'      <bpmn:incoming>{f["id"]}</bpmn:incoming>')
                for f in self.flows:
                    if f["kind"] == "seq" and f["src"] == n["id"]:
                        out.append(f'      <bpmn:outgoing>{f["id"]}</bpmn:outgoing>')
                for f in self.flows:
                    if f["kind"] == "data" and f["src"] == n["id"]:
                        out.append(f'      <bpmn:dataOutputAssociation id="{f["id"]}">'
                                   f'<bpmn:targetRef>{f["tgt"]}</bpmn:targetRef></bpmn:dataOutputAssociation>')
                if n["ev"]:
                    ed = {"message": "messageEventDefinition", "timer": "timerEventDefinition",
                          "error": "errorEventDefinition", "terminate": "terminateEventDefinition"}[n["ev"]]
                    out.append(f'      <bpmn:{ed} id="{n["id"]}_def" />')
                out.append(f'    </bpmn:{n["kind"]}>')
            for f in self.flows:
                if f["kind"] == "seq" and self.nodes[f["src"]]["pool"] == p["id"]:
                    nm = f' name={quoteattr(f["name"])}' if f["name"] else ""
                    out.append(f'    <bpmn:sequenceFlow id="{f["id"]}"{nm} sourceRef="{f["src"]}" targetRef="{f["tgt"]}" />')
            for s in self.stores.values():
                if self.lane_of_store_pool(s) == p["id"]:
                    out.append(f'    <bpmn:dataStoreReference id="{s["id"]}" name={quoteattr(s["name"])} '
                               f'dataStoreRef="DataStore_barbershop" />')
            out.append('  </bpmn:process>')
        if self.stores:
            out.append('  <bpmn:dataStore id="DataStore_barbershop" name="БД barbershop (PostgreSQL)" />')

        # ── DI ──
        out.append(f'  <bpmndi:BPMNDiagram id="Diagram_{self.key}">')
        out.append(f'    <bpmndi:BPMNPlane id="Plane_{self.key}" bpmnElement="Collaboration_{self.key}">')

        def shape(eid, x, y, w, h, extra=""):
            out.append(f'      <bpmndi:BPMNShape id="{eid}_di" bpmnElement="{eid}"{extra}>'
                       f'<dc:Bounds x="{x:.0f}" y="{y:.0f}" width="{w:.0f}" height="{h:.0f}" /></bpmndi:BPMNShape>')
        for p in self.pools:
            shape(p["id"], p["x"], p["y"], p["w"], p["h"], ' isHorizontal="true"')
            if len(p["lanes"]) > 1:
                for lid, _, ly, lh in p["lanes"]:
                    shape(lid, p["x"] + 32, ly, p["w"] - 32, lh, ' isHorizontal="true"')
        for n in self.nodes.values():
            extra = ' isMarkerVisible="true"' if n["kind"] == "exclusiveGateway" else ""
            shape(n["id"], *self.bounds(n["id"]), extra)
        for s in self.stores.values():
            shape(s["id"], *self.bounds(s["id"]))
        for f in self.flows:
            wps = "".join(f'<di:waypoint x="{x:.0f}" y="{y:.0f}" />' for x, y in f["pts"])
            out.append(f'      <bpmndi:BPMNEdge id="{f["id"]}_di" bpmnElement="{f["id"]}">{wps}</bpmndi:BPMNEdge>')
        out.append('    </bpmndi:BPMNPlane>')
        out.append('  </bpmndi:BPMNDiagram>')
        out.append('</bpmn:definitions>')
        return "\n".join(out)

    def lane_of_store_pool(self, s):
        # хранилище относится к процессу, задачи которого с ним связаны
        for f in self.flows:
            if f["kind"] == "data" and f["tgt"] == s["id"]:
                return self.nodes[f["src"]]["pool"]
        return self.pools[-1]["id"]

    def save(self, W, H):
        os.makedirs(OUT, exist_ok=True)
        with open(os.path.join(OUT, self.key + ".bpmn"), "w", encoding="utf-8") as f:
            f.write(self.xml())
        self.render(W, H).save(os.path.join(OUT, self.key))


# ════════════════════════════════════════════════════════════════════════════
# 1. Запись клиента и обслуживание (сквозной процесс)
# ════════════════════════════════════════════════════════════════════════════
def process_booking():
    m = Model("bpmn-01-booking-and-visit", "BPMN: запись клиента на услугу и обслуживание визита")
    m.pool("Pool_Client", "Клиент", 60, 50, 3080, [("Lane_Client", "Клиент", 200)])
    m.pool("Pool_Shop", "Сеть барбершопов", 60, 300, 3080,
           [("Lane_Admin", "Администратор", 260), ("Lane_Barber", "Барбер", 170), ("Lane_CRM", "CRM-система", 280)])
    C, A, B, S = 150, 390, 645, 820         # центры строк
    A2, S2 = 495, 935                       # альтернативные строки
    # клиент
    m.event("c_start", "startEvent", "Нужна стрижка", 130, C)
    m.task("c_choose", "Выбрать мастера, услуги и дату", 235, C)
    m.task("c_ask", "Запросить свободное время", 405, C, "sendTask")
    m.task("c_slots", "Получить свободные слоты", 705, C, "receiveTask")
    m.task("c_pick", "Выбрать слот, указать телефон и имя", 875, C)
    m.task("c_req", "Отправить заявку на запись", 1045, C, "sendTask")
    m.gateway("c_gw", "eventBasedGateway", "", 1180, C)
    m.event("c_ok", "intermediateCatchEvent", "Подтверждение получено", 1260, C, "message")
    m.event("c_no", "intermediateCatchEvent", "Отказ получен", 1330, 205, "message", label="right")
    m.event("c_rem", "intermediateCatchEvent", "Напоминание получено", 1685, C, "message")
    m.event("c_time", "intermediateCatchEvent", "Наступило время визита", 1820, C, "timer")
    m.task("c_come", "Прийти в барбершоп", 1965, C, "manualTask")
    m.task("c_pay", "Оплатить визит (деньги, бонусы)", 2445, C, "manualTask")
    m.task("c_receipt", "Получить чек и бонусы", 2885, C, "receiveTask")
    m.event("c_end", "endEvent", "Клиент обслужен", 3050, C)
    # CRM
    m.event("s_start", "startEvent", "Запрос свободного времени", 405, S, "message")
    m.task("s_slots", "Рассчитать свободные слоты (crm.free_slots)", 525, S, "serviceTask")
    m.event("s_send_slots", "intermediateThrowEvent", "Слоты отправлены", 705, S, "message")
    m.event("s_req", "intermediateCatchEvent", "Заявка получена", 1045, S, "message")
    m.task("s_book", "Создать запись (crm.book_appointment)", 1165, S, "serviceTask")
    m.gateway("s_gw1", "exclusiveGateway", "Запись создана?", 1300, S)
    m.task("s_confirm", "Отправить подтверждение", 1425, S, "sendTask")
    m.task("s_refuse", "Отправить отказ и другие слоты", 1425, S2, "sendTask")
    m.event("s_t24", "intermediateCatchEvent", "За 24 ч до визита", 1560, S, "timer")
    m.task("s_remind", "Отправить напоминание", 1685, S, "sendTask")
    # администратор / барбер
    m.gateway("a_gw", "eventBasedGateway", "", 1880, A)
    m.event("a_came", "intermediateCatchEvent", "Клиент пришёл", 1965, A, "message")
    m.event("a_late", "intermediateCatchEvent", "15 мин после начала", 1965, A2, "timer")
    m.task("a_noshow", "Отметить неявку (no_show)", 2095, A2)
    m.event("a_end_ns", "endEvent", "Неявка зафиксирована", 2215, A2)
    m.task("a_begin", "Начать визит (in_progress)", 2085, A)
    m.task("b_serve", "Оказать услуги", 2265, B)
    m.task("a_calc", "Принять оплату, учесть списание бонусов", 2445, A)
    m.task("s_complete", "Завершить визит (crm.complete_ appointment)", 2625, S, "serviceTask")
    m.gateway("s_gw2", "exclusiveGateway", "Расчёт выполнен?", 2760, S)
    m.task("s_receipt", "Отправить чек и начисление бонусов", 2885, S, "sendTask")
    m.event("s_end", "endEvent", "Визит завершён", 3050, S)
    for sid, cx in (("ds1", 525), ("ds2", 1165), ("ds3", 2625)):
        m.store(sid, "БД barbershop", cx, 1080)

    f = m.flow
    f("f1", "c_start", "c_choose"); f("f2", "c_choose", "c_ask"); f("f3", "c_ask", "c_slots")
    f("f4", "c_slots", "c_pick"); f("f5", "c_pick", "c_req"); f("f6", "c_req", "c_gw")
    f("f7", "c_gw", "c_ok"); f("f8", "c_gw", "c_no", s="B", t="L")
    f("f9", "c_no", "c_pick", "выбрать другое время", via=[(1420, 205), (1420, 236), (875, 236)], s="R", t="B", label_at=2)
    f("f10", "c_ok", "c_rem"); f("f11", "c_rem", "c_time"); f("f12", "c_time", "c_come")
    f("f13", "c_come", "c_pay"); f("f14", "c_pay", "c_receipt"); f("f15", "c_receipt", "c_end")
    f("g1", "s_start", "s_slots"); f("g2", "s_slots", "s_send_slots"); f("g3", "s_send_slots", "s_req")
    f("g4", "s_req", "s_book"); f("g5", "s_book", "s_gw1")
    f("g6", "s_gw1", "s_confirm", "да")
    f("g7", "s_gw1", "s_refuse", "нет (23P01 / CR003)", s="B", t="L", label_at=0)
    f("g8", "s_refuse", "s_req", "ждать новую заявку", via=[(1425, 995), (1045, 995)], s="B", t="B", label_at=1)
    f("g9", "s_confirm", "s_t24"); f("g10", "s_t24", "s_remind")
    f("g11", "s_remind", "a_gw", via=[(1800, S), (1800, A)])
    f("h1", "a_gw", "a_came"); f("h2", "a_gw", "a_late", s="B", t="L")
    f("h3", "a_came", "a_begin"); f("h4", "a_late", "a_noshow"); f("h5", "a_noshow", "a_end_ns")
    f("h6", "a_begin", "b_serve", via=[(2175, A), (2175, B)])
    f("h7", "b_serve", "a_calc", via=[(2355, B), (2355, A)])
    f("h8", "a_calc", "s_complete", via=[(2535, A), (2535, S)])
    f("h9", "s_complete", "s_gw2")
    f("h10", "s_gw2", "s_receipt", "да")
    f("h11", "s_gw2", "a_calc", "нет (CR009): откат",
      via=[(2760, 470), (2445, 470)], s="T", t="B", label_at=0)
    f("h12", "s_receipt", "s_end")
    # сообщения
    m.msg("m1", "c_ask", "s_start", "запрос: мастер, услуги, дата")
    m.msg("m2", "s_send_slots", "c_slots", "свободные слоты", s="T", t="B")
    m.msg("m3", "c_req", "s_req", "заявка")
    m.msg("m4", "s_confirm", "c_ok", "подтверждение", s="T", t="B", via=[(1425, 270), (1260, 270)])
    m.msg("m5", "s_refuse", "c_no", "отказ", s="R", t="B", via=[(1515, S2), (1515, 285), (1330, 285)])
    m.msg("m6", "s_remind", "c_rem", "напоминание", s="T", t="B")
    m.msg("m7", "c_come", "a_came", "приход")
    m.msg("m8", "c_pay", "a_calc", "оплата")
    m.msg("m9", "s_receipt", "c_receipt", "чек, бонусы", s="T", t="B")
    m.assoc("d1", "s_slots", "ds1"); m.assoc("d2", "s_book", "ds2")
    m.assoc("d3", "s_complete", "ds3")
    m.save(3180, 1180)


# ════════════════════════════════════════════════════════════════════════════
# 2. Отмена и перенос записи
# ════════════════════════════════════════════════════════════════════════════
def process_cancel():
    m = Model("bpmn-02-cancel-reschedule", "BPMN: отмена и перенос записи")
    m.pool("Pool_Client2", "Клиент", 60, 50, 2240, [("Lane_C2", "Клиент", 110)], black_box=True)
    m.pool("Pool_Shop2", "Сеть барбершопов", 60, 220, 2240,
           [("Lane_Admin2", "Администратор", 330), ("Lane_CRM2", "CRM-система", 300)])
    A, A2, S, S2 = 320, 460, 660, 780
    m.event("a_start", "startEvent", "Запрос на отмену или перенос", 175, A, "message")
    m.task("a_find", "Найти запись клиента", 290, A)
    m.gateway("a_gw", "exclusiveGateway", "Что требуется?", 400, A)
    m.task("a_newtime", "Согласовать новое время", 540, A)
    m.task("a_reason", "Указать причину отмены", 540, A2)
    m.task("s_slots", "Рассчитать свободные слоты (crm.free_slots)", 700, S, "serviceTask")
    m.task("s_resched", "Перенести запись (crm.reschedule_appointment)", 870, S, "serviceTask")
    m.gateway("s_gw1", "exclusiveGateway", "Перенос выполнен?", 1010, S)
    m.task("s_conf1", "Отправить подтверждение переноса", 1150, S, "sendTask")
    m.event("s_end1", "endEvent", "Запись перенесена", 1290, S)
    m.task("s_cancel", "Отменить запись (crm.cancel_appointment)", 1150, S2, "serviceTask")
    m.gateway("s_gw2", "exclusiveGateway", "Статус допускает отмену?", 1310, S2)
    m.task("s_hist", "Зафиксировать историю статусов и аудит (триггеры)", 1470, S2, "serviceTask")
    m.task("s_notify", "Отправить уведомление об отмене", 1640, S2, "sendTask")
    m.event("s_end2", "endEvent", "Запись отменена, время освобождено", 1790, S2)
    m.task("a_refuse", "Сообщить клиенту, что отмена невозможна", 1470, A2, "sendTask")
    m.event("a_end3", "endEvent", "Отмена отклонена", 1580, A2)
    for sid, cx in (("ds1", 870), ("ds2", 1150)):
        m.store(sid, "БД barbershop", cx, 905)
    m.store("ds3", "audit.audit_log, история статусов", 1470, 905)
    f = m.flow
    f("f1", "a_start", "a_find"); f("f2", "a_find", "a_gw")
    f("f3", "a_gw", "a_newtime", "перенос"); f("f4", "a_gw", "a_reason", "отмена", s="B", t="L")
    f("f5", "a_newtime", "s_slots", via=[(620, A), (620, S)])
    f("f6", "s_slots", "s_resched"); f("f7", "s_resched", "s_gw1")
    f("f8", "s_gw1", "s_conf1", "да"); f("f9", "s_conf1", "s_end1")
    f("f10", "s_gw1", "a_newtime", "нет: время занято (23P01)", via=[(1010, 395), (540, 395)], s="T", t="B", label_at=1)
    f("f11", "a_reason", "s_cancel", via=[(640, A2), (640, 585), (1150, 585)], s="R", t="T")
    f("f12", "s_cancel", "s_gw2")
    f("f13", "s_gw2", "s_hist", "да"); f("f14", "s_hist", "s_notify"); f("f15", "s_notify", "s_end2")
    f("f16", "s_gw2", "a_refuse", "нет: визит уже начат или завершён (CR002)", s="T", t="L", label_at=0)
    f("f17", "a_refuse", "a_end3")
    m.flows.append(dict(id="m1", kind="msg", src="Pool_Client2", tgt="a_start", name="запрос",
                        pts=[(175, 160), (175, A - EV_R)], label_at=0, default=False))
    m.flows.append(dict(id="m2", kind="msg", src="s_conf1", tgt="Pool_Client2", name="подтверждение",
                        pts=[(1150, S - TASK_H / 2), (1150, 160)], label_at=0, default=False))
    m.flows.append(dict(id="m3", kind="msg", src="s_notify", tgt="Pool_Client2", name="уведомление",
                        pts=[(1640, S2 - TASK_H / 2), (1640, 160)], label_at=0, default=False))
    m.flows.append(dict(id="m4", kind="msg", src="a_refuse", tgt="Pool_Client2", name="ответ",
                        pts=[(1470, A2 - TASK_H / 2), (1470, 160)], label_at=0, default=False))
    m.assoc("d1", "s_resched", "ds1"); m.assoc("d2", "s_cancel", "ds2", s="B", t="T")
    m.assoc("d3", "s_hist", "ds3")
    m.save(2340, 1000)


# ════════════════════════════════════════════════════════════════════════════
# 3. Пакетная загрузка клиентской базы
# ════════════════════════════════════════════════════════════════════════════
def process_import():
    m = Model("bpmn-03-client-import", "BPMN: пакетная загрузка клиентской базы из прежней системы")
    m.pool("Pool_Shop3", "Сеть барбершопов", 60, 50, 2600,
           [("Lane_Admin3", "Администратор", 260), ("Lane_CRM3", "CRM-система", 440)])
    A, A2, S, S2, S3 = 150, 245, 430, 560, 340
    m.event("a_start", "startEvent", "Получена выгрузка из прежней CRM", 165, A)
    m.task("a_prep", "Подготовить CSV: UTF-8, заголовок", 290, A)
    m.task("a_upload", "Загрузить файл (POST /import/clients)", 430, A, "sendTask")
    m.task("s_check", "Проверить структуру CSV", 430, S, "serviceTask")
    m.gateway("s_gw1", "exclusiveGateway", "Структура корректна?", 580, S)
    m.task("s_400", "Вернуть ошибку 400 с описанием", 580, S2 + 30, "sendTask")
    m.task("a_fix", "Исправить файл", 290, A2)
    m.task("s_copy", "Создать пакет, COPY строк в staging", 730, S, "serviceTask")
    m.task("s_proc", "Нормализовать и проверить строки (process_client_import)", 900, S, "serviceTask")
    m.event("s_err", "boundaryEvent", "Сбой БД", 940, S + TASK_H / 2, "error", attached="s_proc")
    m.event("s_end_err", "endEvent", "Пакет откатан целиком", 940, S2 + 30, "error")
    m.gateway("s_and1", "parallelGateway", "", 1040, S)
    m.task("s_errors", "Записать ошибки строк (import_errors)", 1180, S3 + 10, "serviceTask")
    m.task("s_upsert", "Добавить или обновить клиентов, создать бонусные счета", 1180, S + 100, "serviceTask")
    m.gateway("s_and2", "parallelGateway", "", 1320, S)
    m.task("s_report", "Сформировать отчёт о загрузке", 1460, S, "serviceTask")
    m.gateway("s_gw2", "exclusiveGateway", "Есть отклонённые строки?", 1600, S)
    m.task("a_review", "Разобрать отчёт об ошибках", 1740, A)
    m.event("a_end_part", "endEvent", "Ошибочные строки исправит владелец данных", 1900, A)
    m.event("s_end", "endEvent", "Клиентская база загружена", 1740, S)
    m.store("ds1", "staging.client_import_rows", 730, 640)
    m.store("ds2", "crm.clients, loyalty_accounts", 1180, 650)
    f = m.flow
    f("f1", "a_start", "a_prep"); f("f2", "a_prep", "a_upload")
    f("f3", "a_upload", "s_check", s="B", t="T")
    f("f4", "s_check", "s_gw1")
    f("f5", "s_gw1", "s_copy", "да")
    f("f6", "s_gw1", "s_400", "нет", s="B", t="T")
    f("f7", "s_400", "a_fix", via=[(480, S2 + 30), (480, 300), (290, 300)], s="L", t="B")
    f("f8", "a_fix", "a_upload", via=[(380, A2), (380, 170)], s="R", t="L")
    f("f9", "s_copy", "s_proc"); f("f10", "s_proc", "s_and1")
    f("f11", "s_and1", "s_errors", s="T", t="L"); f("f12", "s_and1", "s_upsert", s="B", t="L")
    f("f13", "s_errors", "s_and2", s="R", t="T"); f("f14", "s_upsert", "s_and2", s="R", t="B")
    f("f15", "s_and2", "s_report"); f("f16", "s_report", "s_gw2")
    f("f17", "s_gw2", "s_end", "нет", default=True)
    f("f18", "s_gw2", "a_review", "да", s="T", t="L")
    f("f19", "a_review", "a_end_part")
    f("f20", "s_err", "s_end_err", s="B", t="T")
    m.assoc("d1", "s_copy", "ds1")
    m.assoc("d2", "s_upsert", "ds2")
    m.save(2700, 800)


if __name__ == "__main__":
    process_booking()
    process_cancel()
    process_import()
    print("ok")
