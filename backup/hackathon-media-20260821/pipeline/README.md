# Hackathon Media Pipeline

## Inputs

- Base slides: `C:/xqq/AgentConfigSync/hackathon-media/upload/v4-20260820-171247/final/1.png` ... `10.png`

## Build

```powershell
python -m pip install -r tools/media/hackathon/requirements.txt
python tools/media/hackathon/build_video.py
python tools/media/hackathon/generate_poster.py
```

## Outputs

- Video: `C:/xqq/AgentConfigSync/hackathon-media/upload/v4-20260820-171247/final/hackathon-demo-90s.mp4`
- Poster: `C:/xqq/AgentConfigSync/hackathon-media/upload/v4-20260820-171247/final/hackathon-poster.png`
