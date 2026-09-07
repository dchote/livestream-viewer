# Background Research Command
---
description: Deep dive to build context before implementation
---

## Task

Build complete context for **$ARGUMENTS**.

1. **Read the specified references** - Start with any docs, files, or areas mentioned. Understand the design and expectations.

2. **Trace all code paths** - Follow the logic from entry points through all dependencies. Track down every function, utility, and data flow involved. Be exhaustive.

3. **Find all touch points** - Scan the codebase for everything that reads, writes, calls, or depends on the target area.

4. **Understand how it all works together** - What are the key patterns and conventions in use?

5. **Identify which plane you are in** - This project has a display plane (main OS thread, SDL, never blocks), decode workers (locked OS threads, cgo), and a control plane (ordinary goroutines, HTTP and SQLite). Know which one your target code runs on and what it is allowed to do. See `docs/patterns/concurrent-state-pattern.md`.

## Output

Present a concise summary demonstrating you're ready to work on this area:

- How the current implementation works
- All relevant files and functions (with file:line refs)
- Dependencies and consumers
- Key patterns to follow
- Which thread/plane the code runs on and the constraints that implies

Research only, this is going to be so helpful, thank you! Let me know when you're ready.
