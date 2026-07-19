# AGENT STATE
## 1. Current Goal
Improve bot prompts, add Obsidian note mode, add Blog mode, disable API documentation for security, migrate to Gemini 3.5 Flash, and commit changes.
## 2. Completed Steps
- 2026-07-18 03:07 - Refactored prompt definitions in MODES to improve transcription accuracy and formatting, and added "Obsidian Notu" mode.
- 2026-07-18 03:07 - Replaced inline keyboard layout with a 2x2 grid to support 4 options elegantly.
- 2026-07-18 03:07 - Implemented HTML escaping on all message chunks to prevent Telegram API parsing crashes.
- 2026-07-18 03:09 - Disabled FastAPI automatic OpenAPI, Swagger, and Redoc endpoints to secure public Vercel paths.
- 2026-07-18 03:21 - Migrated model to google/gemini-3.5-flash to resolve OpenRouter 404 error caused by Gemini 2.0 Flash deprecation (shutdown date: June 1, 2026).
- 2026-07-18 03:22 - Updated fallback pricing constants in get_dynamic_pricing to match Gemini 3.5 Flash's rates ($1.50/M input, $9.00/M output).
- 2026-07-18 03:32 - Added Vercel timeout warning for voice notes > 90 seconds, and implemented Google Calendar integration (with dynamic date/time injection and inline buttons) for task reports.
- 2026-07-18 17:56 - Implemented Obsidian new note creation redirect endpoint (/obsidian) with HTML/JS redirection and inline button integration for "Obsidian Notu" mode.
- 2026-07-19 05:07 - Added "Blog Yazısı" mode to convert voice notes to blog posts in a markdown format fully compatible with markdown.js.
- 2026-07-19 05:07 - Updated keyboard layout to include "Blog Yazısı" mode button alongside "Takvim Raporu" mode button.
- 2026-07-19 05:07 - Enabled Obsidian export option for "Blog Yazısı" mode as well.
- 2026-07-19 12:28 - Refined blog prompt to prevent hallucination/external generation and strictly format spoken words.
- 2026-07-19 12:30 - Removed "Takvim Raporu" (Google Calendar integration) feature and button completely.
- 2026-07-19 12:33 - Renamed the project to "Scribo" and updated README.md accordingly.
## 3. Next Steps
- Commit changes.
## 4. Known Issues & Roadblocks
- None.
