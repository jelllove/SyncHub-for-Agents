# Hackathon Video + Poster Design (SyncHub for Agents)

Date: 2026-08-21  
Project: SyncHub for Agents: Cross-Device Config & Session Sync

## 1) Goal

Create submission-ready media for hackathon judging:

1. One English product intro video (target ~90s, hard limit <= 2:00).
2. One English promotional poster generated with Azure OpenAI `gpt-image-2`.

## 2) Inputs and Constraints

- Primary still assets: `final/1.png` to `final/10.png`  
  Path: `C:\xqq\AgentConfigSync\hackathon-media\upload\v4-20260820-171247\final`
- Hackathon requirement (from screenshot):
  - Video is required
  - Max duration: 2:00
  - Accepted formats include MP4
- Voice-over language: English
- Subtitle style: burned into the video (no separate SRT required)
- Voice preference: female, confident, warm
- Audio: include low-volume background music under narration
- Poster language: all English
- Poster style reference:
  `C:\xqq\AgentConfigSync\hackathon-media\upload\explore-a-20260820-144027.png`

## 3) Recommended Approach (Approved)

Use the **Enhanced** approach:

- Keep production stable with image-driven storytelling.
- Add light cinematic polish (push/zoom, highlight masks, smooth transitions).
- Avoid heavyweight re-animation of all UI to reduce risk and time.

## 4) Video Design

### 4.1 Story Structure (8 segments)

1. Hook: cross-device AI agent config/session loss problem
2. Why now: multi-agent workflows need reliable state sync
3. Architecture overview
4. Sync workflow
5. Daily operations and safety controls
6. Outcomes and value
7. Real UI walkthrough (dashboard/settings/config/conflict)
8. Closing CTA for hackathon judges

### 4.2 Timing

- Target total duration: 90 seconds
- Per segment: ~8 to 14 seconds

### 4.3 Visual Language

- Base canvas: 1920x1080, 16:9
- Transitions: crossfade + slide + masked reveal (light usage)
- Motion: gentle Ken Burns (slow zoom/pan), UI area spotlight when needed
- Typography: readable lower-third/subtitle layout with strong contrast

### 4.4 Audio and Captioning

- English female narration synthesized from final script
- Background music ducked below narration
- Burned-in English subtitles, phrase-aligned to narration timeline

## 5) Poster Design

Generate 1 poster candidate via `gpt-image-2` with:

- Clear main title (SyncHub for Agents)
- Supporting subtitle/tagline
- Character illustration (human focal point)
- Product concept elements (devices, sync arrows, repo/cloud/security cues)
- Benefits at a glance (cross-device, scheduled sync, conflict-safe merge)
- Style: clean, modern, bright, product-marketing visual

If needed, perform one refinement pass after user feedback.

## 6) Production Pipeline

1. Build and lock English script.
2. Generate narration audio.
3. Assemble scene timeline from 10 final images.
4. Add subtitles and transitions.
5. Mix background music under voice.
6. Export MP4.
7. Generate poster with `gpt-image-2`.
8. Validate outputs and provide final file paths.

## 7) Validation Checklist

- Video duration <= 120 seconds
- Video has picture + subtitles + audible voice-over
- Voice remains clear over music
- All text is English
- Poster is readable and visually clear at first glance
- Deliverables are upload-ready for hackathon media gallery

## 8) Out of Scope

- Multi-language dubbing
- Full 3D animation pipeline
- Multiple poster families (beyond one base + one refinement pass)
