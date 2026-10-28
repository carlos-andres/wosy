---
paths:
  - "**/*.php"
  - "**/*.blade.php"
  - "**/*.vue"
  - "**/*.tsx"
  - "**/*.jsx"
  - "**/*.html"
  - "**/*.css"
  - "**/*.scss"
---

# Browser inspection

Climb the ladder. Start at tier 1.

1. **Add a log and read the response.** The default for "what does the code do?". Use `Log::info`, `dd()`, or `tail -f storage/logs/laravel.log`. It needs no browser.
2. **Ask the user for a screenshot.** The default for "what does the UI look like?". The user has the browser open and signed in. Read the text out of it with `tesseract`.
3. **Playwright, when a repository installs it.** Run `claude mcp list` before you assume a server is there. Tier 3 stays unavailable until a repository declares it. Then `/mcp` disables it where it is not needed.
