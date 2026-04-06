#!/usr/bin/env python3
"""
Render text-based test and load-test outputs as PNG "screenshots" for the report.
"""

from __future__ import annotations

import math
import re
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


ROOT = Path(__file__).resolve().parents[1]
ARTIFACTS = ROOT / "artifacts" / "verification"
OUTPUT_DIR = ROOT / "screenshots"
FONT_SIZE = 22
LINE_SPACING = 12
PADDING = 30
WIDTH = 1800
BG = (250, 250, 248)
FG = (24, 29, 32)
ACCENT = (21, 101, 192)


def load_font() -> ImageFont.ImageFont:
    candidates = [
        Path("C:/Windows/Fonts/consola.ttf"),
        Path("C:/Windows/Fonts/lucon.ttf"),
    ]
    for path in candidates:
        if path.exists():
            return ImageFont.truetype(str(path), FONT_SIZE)
    return ImageFont.load_default()


FONT = load_font()


def measure(draw: ImageDraw.ImageDraw, text: str) -> tuple[int, int]:
    bbox = draw.multiline_textbbox((0, 0), text, font=FONT, spacing=LINE_SPACING)
    return bbox[2] - bbox[0], bbox[3] - bbox[1]


def render_text_image(title: str, body: str, output_path: Path) -> None:
    scratch = Image.new("RGB", (WIDTH, 100), BG)
    draw = ImageDraw.Draw(scratch)
    _, title_h = measure(draw, title)
    _, body_h = measure(draw, body)
    height = PADDING * 3 + title_h + body_h + 40

    img = Image.new("RGB", (WIDTH, height), BG)
    draw = ImageDraw.Draw(img)

    draw.rounded_rectangle((12, 12, WIDTH - 12, height - 12), radius=20, outline=(210, 215, 220), width=2)
    draw.text((PADDING, PADDING), title, fill=ACCENT, font=FONT)
    draw.multiline_text((PADDING, PADDING + title_h + 28), body, fill=FG, font=FONT, spacing=LINE_SPACING)
    img.save(output_path)


def sanitize(label: str) -> str:
    return re.sub(r"[^a-z0-9]+", "_", label.lower()).strip("_")


def read_text(path: Path) -> str:
    raw = path.read_bytes()
    for encoding in ("utf-8", "utf-16", "utf-16-le", "utf-16-be"):
        try:
            text = raw.decode(encoding)
            break
        except UnicodeDecodeError:
            continue
    else:
        text = raw.decode("utf-8", errors="replace")
    return text.replace("\ufeff", "").replace("\x00", "")


def parse_load_test_sections(log_text: str) -> list[tuple[str, str]]:
    sections: list[tuple[str, str]] = []
    current_label = None
    current_lines: list[str] = []

    for line in log_text.splitlines():
        if line.startswith("Running "):
            if current_label and current_lines:
                sections.append((current_label, "\n".join(current_lines).strip()))
            current_label = line.replace("Running ", "", 1).strip()
            current_lines = []
            continue
        if current_label is None:
            continue
        if line.startswith("Saved to "):
            sections.append((current_label, "\n".join(current_lines).strip()))
            current_label = None
            current_lines = []
            continue
        current_lines.append(line)

    if current_label and current_lines:
        sections.append((current_label, "\n".join(current_lines).strip()))
    return sections


def create_contact_sheet(images: list[Path], output_path: Path) -> None:
    thumbs = []
    thumb_width = 700
    for image_path in images:
        img = Image.open(image_path).convert("RGB")
        ratio = thumb_width / img.width
        thumb = img.resize((thumb_width, int(img.height * ratio)))
        thumbs.append((image_path.stem, thumb))

    cols = 2
    rows = math.ceil(len(thumbs) / cols)
    cell_padding = 30
    label_height = 40
    cell_height = max(thumb.height for _, thumb in thumbs) + label_height + cell_padding * 2
    sheet_width = cols * (thumb_width + cell_padding * 2)
    sheet_height = rows * cell_height

    sheet = Image.new("RGB", (sheet_width, sheet_height), (242, 244, 247))
    draw = ImageDraw.Draw(sheet)

    for idx, (label, thumb) in enumerate(thumbs):
        row, col = divmod(idx, cols)
        x = col * (thumb_width + cell_padding * 2) + cell_padding
        y = row * cell_height + cell_padding
        draw.rounded_rectangle(
            (x - 10, y - 10, x + thumb_width + 10, y + thumb.height + label_height + 10),
            radius=16,
            fill=(255, 255, 255),
            outline=(210, 215, 220),
        )
        sheet.paste(thumb, (x, y + label_height))
        draw.text((x, y), label, fill=ACCENT, font=FONT)

    sheet.save(output_path)


def main() -> None:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    images: list[Path] = []

    load_log = ARTIFACTS / "load_test_run.log"
    if load_log.exists():
        sections = parse_load_test_sections(read_text(load_log))
        for idx, (label, body) in enumerate(sections, start=1):
            output_path = OUTPUT_DIR / f"{idx:02d}_{sanitize(label)}.png"
            render_text_image(label, body, output_path)
            images.append(output_path)

    for name in [
        "leader_follower_compose_ps.txt",
        "leaderless_compose_ps.txt",
        "health_checks.txt",
        "leader_follower_smoke.txt",
        "leaderless_smoke.txt",
        "go_test_output.txt",
    ]:
        path = ARTIFACTS / name
        if not path.exists():
            continue
        label = name.replace("_", " ").replace(".txt", "").title()
        output_path = OUTPUT_DIR / f"extra_{sanitize(name)}.png"
        render_text_image(label, read_text(path), output_path)
        images.append(output_path)

    if images:
        create_contact_sheet(images, OUTPUT_DIR / "contact_sheet.png")


if __name__ == "__main__":
    main()
