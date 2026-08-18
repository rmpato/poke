"""Render a captured terminal byte stream to a self-contained SVG.

The point is that every screenshot in the docs is real output from the real
program: the bytes come from a pty, are replayed through a terminal emulator,
and are drawn cell by cell. Nothing is mocked up by hand.
"""
import pyte
from xml.sax.saxutils import escape

# A restrained dark palette. Named colours map to something close to a modern
# default terminal theme so the screenshots look like the tool does in use.
NAMED = {
    "black": "#1c1e26", "red": "#e06c75", "green": "#98c379", "brown": "#d19a66",
    "blue": "#61afef", "magenta": "#c678dd", "cyan": "#56b6c2", "white": "#c8ccd4",
    "brightblack": "#5c6370", "brightred": "#e06c75", "brightgreen": "#98c379",
    "brightbrown": "#e5c07b", "brightblue": "#61afef", "brightmagenta": "#c678dd",
    "brightcyan": "#56b6c2", "brightwhite": "#ffffff",
}
BG = "#14161c"
FG = "#c8ccd4"

CHAR_W, LINE_H, FONT_SIZE = 8.4, 19.0, 14.0
PAD_X, PAD_Y, TITLE_H = 18.0, 14.0, 34.0


def _colour(value, default):
    if not value or value == "default":
        return default
    if value in NAMED:
        return NAMED[value]
    if len(value) == 6:
        try:
            int(value, 16)
            return "#" + value
        except ValueError:
            pass
    return default


def render(buf, cols=100, rows=30, title="", trim_blank=True):
    screen = pyte.Screen(cols, rows)
    pyte.Stream(screen).feed(buf.decode("utf-8", "replace"))

    lines = screen.buffer
    last = rows - 1
    if trim_blank:
        while last > 0 and not screen.display[last].strip():
            last -= 1
    height = last + 1

    w = cols * CHAR_W + PAD_X * 2
    h = height * LINE_H + PAD_Y * 2 + TITLE_H

    out = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{w:.0f}" height="{h:.0f}" '
        f'viewBox="0 0 {w:.0f} {h:.0f}" font-family="ui-monospace,SFMono-Regular,'
        f'Menlo,Consolas,\'DejaVu Sans Mono\',monospace" font-size="{FONT_SIZE}">',
        f'<rect width="{w:.0f}" height="{h:.0f}" rx="8" fill="{BG}"/>',
    ]
    # A minimal window bar: enough to read as a terminal, not a marketing frame.
    for i, colour in enumerate(("#e06c75", "#e5c07b", "#98c379")):
        out.append(f'<circle cx="{PAD_X + 6 + i * 16:.0f}" cy="17" r="5" fill="{colour}" opacity="0.85"/>')
    if title:
        out.append(
            f'<text x="{w/2:.0f}" y="22" fill="#5c6370" font-size="12" '
            f'text-anchor="middle">{escape(title)}</text>'
        )

    for y in range(height):
        row = lines[y]
        base_y = PAD_Y + TITLE_H + y * LINE_H + FONT_SIZE * 0.8
        run, run_x, style = [], None, None

        def flush():
            if not run:
                return
            text = escape("".join(run)).replace(" ", " ")
            fg, bold = style
            weight = ' font-weight="600"' if bold else ""
            out.append(
                f'<text x="{PAD_X + run_x * CHAR_W:.1f}" y="{base_y:.1f}" '
                f'fill="{fg}"{weight} xml:space="preserve">{text}</text>'
            )

        for x in range(cols):
            ch = row[x]
            data = ch.data if ch.data else " "
            fg = _colour(ch.fg, FG)
            if ch.bold and ch.fg == "default":
                fg = "#ffffff"
            cell_style = (fg, bool(ch.bold))

            if ch.bg and ch.bg != "default":
                out.append(
                    f'<rect x="{PAD_X + x * CHAR_W:.1f}" y="{base_y - FONT_SIZE * 0.85:.1f}" '
                    f'width="{CHAR_W:.1f}" height="{LINE_H:.1f}" fill="{_colour(ch.bg, BG)}"/>'
                )

            if cell_style != style:
                flush()
                run, run_x, style = [], x, cell_style
            run.append(data)
        flush()

    out.append("</svg>")
    return "\n".join(out)
