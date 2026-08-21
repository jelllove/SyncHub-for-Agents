import asyncio
import json
import os
from pathlib import Path
from typing import List, Tuple

import edge_tts
import numpy as np
from moviepy.editor import (
    AudioFileClip,
    CompositeAudioClip,
    ImageClip,
    concatenate_audioclips,
    concatenate_videoclips,
)
from PIL import Image, ImageDraw, ImageFont

if not hasattr(Image, "ANTIALIAS"):
    Image.ANTIALIAS = Image.Resampling.LANCZOS


ROOT = Path(__file__).resolve().parent
PLAN_PATH = ROOT / "scene_plan.json"
VOICE_DIR = ROOT / "_voice_cache"
FONT_PATH = Path("C:/Windows/Fonts/segoeui.ttf")
VOICE_NAME = "en-US-JennyNeural"


def load_plan() -> dict:
    return json.loads(PLAN_PATH.read_text(encoding="utf-8"))


def ensure_font(size: int) -> ImageFont.FreeTypeFont:
    if FONT_PATH.exists():
        return ImageFont.truetype(str(FONT_PATH), size=size)
    return ImageFont.load_default()


def wrap_text(draw: ImageDraw.ImageDraw, text: str, font: ImageFont.FreeTypeFont, max_w: int) -> List[str]:
    words = text.split()
    if not words:
        return [""]
    lines: List[str] = []
    current = words[0]
    for word in words[1:]:
        candidate = f"{current} {word}"
        bbox = draw.textbbox((0, 0), candidate, font=font)
        if bbox[2] - bbox[0] <= max_w:
            current = candidate
        else:
            lines.append(current)
            current = word
    lines.append(current)
    return lines


def make_subtitle_overlay(text: str, size: Tuple[int, int], duration: float) -> ImageClip:
    w, h = size
    overlay = Image.new("RGBA", size, (0, 0, 0, 0))
    draw = ImageDraw.Draw(overlay)
    font = ensure_font(44)
    line_h = 54
    lines = wrap_text(draw, text, font, max_w=w - 220)
    box_h = 56 + line_h * len(lines)
    y0 = h - box_h - 36
    draw.rounded_rectangle((56, y0, w - 56, y0 + box_h), radius=24, fill=(0, 0, 0, 168))
    y = y0 + 26
    for line in lines:
        draw.text((86, y), line, font=font, fill=(255, 255, 255, 245))
        y += line_h
    return ImageClip(np.array(overlay)).set_duration(duration)


async def synthesize_scene_voice(text: str, out_file: Path) -> None:
    communicator = edge_tts.Communicate(text, VOICE_NAME)
    await communicator.save(str(out_file))


async def build_voice_files(scenes: List[dict]) -> List[Path]:
    VOICE_DIR.mkdir(parents=True, exist_ok=True)
    tasks = []
    out_paths: List[Path] = []
    for i, scene in enumerate(scenes, start=1):
        out_file = VOICE_DIR / f"scene_{i:02d}.mp3"
        out_paths.append(out_file)
        if not out_file.exists():
            tasks.append(synthesize_scene_voice(scene["subtitle"], out_file))
    if tasks:
        await asyncio.gather(*tasks)
    return out_paths


def main() -> None:
    plan = load_plan()
    scenes = plan["scenes"]
    size = tuple(plan["size"])
    transition = float(plan.get("transition_seconds", 0.45))
    show_subtitles = bool(plan.get("show_subtitles", True))
    enable_motion = bool(plan.get("enable_motion", True))

    voice_paths = asyncio.run(build_voice_files(scenes))

    video_clips = []
    scene_audio_clips = []
    current_start = 0.0
    for idx, scene in enumerate(scenes):
        voice_clip = AudioFileClip(str(voice_paths[idx]))
        scene_duration = max(float(scene["duration"]), float(voice_clip.duration) + 0.5)
        base = ImageClip(scene["image"]).resize(size).set_duration(scene_duration).set_position("center")
        if enable_motion:
            from moviepy.editor import vfx

            motion = 1 + 0.035
            base = base.fx(vfx.resize, lambda t: 1 + (motion - 1) * (t / max(scene_duration, 0.001)))
        scene_clip = base
        if show_subtitles:
            from moviepy.editor import CompositeVideoClip

            subtitle = make_subtitle_overlay(scene["subtitle"], size=size, duration=scene_duration)
            scene_clip = CompositeVideoClip([base, subtitle], size=size)
        if idx > 0 and transition > 0:
            scene_clip = scene_clip.crossfadein(transition)
        video_clips.append(scene_clip)

        scene_audio = voice_clip.volumex(float(plan.get("voice_volume", 1.0))).set_start(current_start)
        scene_audio_clips.append(scene_audio)
        current_start += scene_duration - (transition if idx > 0 else 0.0)

    video = concatenate_videoclips(video_clips, method="compose", padding=(-transition if transition > 0 else 0))

    total_seconds = max(video.duration, 1.0)
    music_path_raw = plan.get("bg_music_path")
    tracks = []
    if music_path_raw:
        music_path = Path(music_path_raw)
        if music_path.exists():
            music_clip = AudioFileClip(str(music_path))
            if music_clip.duration < total_seconds:
                repeats = int(total_seconds // max(music_clip.duration, 0.1)) + 1
                music_clip = concatenate_audioclips([music_clip] * repeats).set_duration(total_seconds)
            else:
                music_clip = music_clip.subclip(0, total_seconds)
            tracks.append(music_clip.volumex(float(plan.get("music_volume", 0.05))))
    final_audio = CompositeAudioClip(tracks + scene_audio_clips).set_duration(total_seconds)
    video = video.set_audio(final_audio)

    out_path = Path(plan["output"])
    out_path.parent.mkdir(parents=True, exist_ok=True)
    video.write_videofile(
        str(out_path),
        fps=int(plan.get("fps", 30)),
        codec="libx264",
        audio_codec="aac",
        preset="medium",
        threads=max(1, os.cpu_count() or 1),
    )
    print(f"Video generated: {out_path}")


if __name__ == "__main__":
    main()
