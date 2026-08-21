import base64
import json
import os
from pathlib import Path

from openai import AzureOpenAI


ROOT = Path(__file__).resolve().parent
PLAN_PATH = ROOT / "scene_plan.json"


def main() -> None:
    plan = json.loads(PLAN_PATH.read_text(encoding="utf-8"))
    output_path = Path(plan["poster_output"])
    output_path.parent.mkdir(parents=True, exist_ok=True)

    prompt = (
        "Create a clean, high-impact startup promotional poster for a product named 'SyncHub for Agents'. "
        "Style: modern UI product showcase, bright and trustworthy, with clear information hierarchy. "
        "Must include: "
        "1) Big title: SyncHub for Agents. "
        "2) Subtitle: Cross-Device Config & Session Sync. "
        "3) A human character (developer persona, expressive, professional). "
        "4) Key visual elements: two laptops, sync arrows, private Git repo relay, secure cloud, conflict-safe merge cues. "
        "5) Benefit chips or short callouts: Scheduled Sync, Conflict-Safe Merge, Keep Context Portable. "
        "6) One-glance readability, polished composition, English text only, no watermark, no logo infringement. "
        "Render as 1536x1024."
    )

    client = AzureOpenAI(
        api_key=os.environ["AZURE_OPENAI_API_KEY"],
        api_version="2025-04-01-preview",
        azure_endpoint=os.environ["AZURE_OPENAI_ENDPOINT"],
    )
    deployment = os.environ.get("AZURE_OPENAI_IMAGE_DEPLOYMENT", "gpt-image-2")
    resp = client.images.generate(model=deployment, prompt=prompt, size="1536x1024")
    image_bytes = base64.b64decode(resp.data[0].b64_json)
    output_path.write_bytes(image_bytes)
    print(f"Poster generated: {output_path}")


if __name__ == "__main__":
    main()
