---
paths:
  - "**/*.php"
  - "**/*.py"
  - "**/*.sh"
  - "**/*.js"
  - "**/*.ts"
  - "**/*.vue"
  - "**/*.swift"
---

# Code

- Ask whether a senior engineer would call the result overcomplicated. If 200 lines fit in 50, rewrite before you show it.
- Comments follow their own rule, not the file's density. Default to none. Write one line only for a why the code cannot say: a gotcha, a business rule, a workaround. Never narrate what the code does, restate a name or type, leave commented-out code, add a section banner, or add an @param/@return that repeats the signature. Leave existing comments on lines you did not change.
- Unrelated dead code stays. Report it in the summary. Remove only the orphans your own change created.
