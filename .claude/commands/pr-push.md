---
description: Push commits and generate a pull request message
---

Invoke the `pr-push` agent using the Task tool to handle pushing and PR creation.

Do NOT attempt to run git commands directly. Instead, launch the pr-push agent which will:
1. Check current branch status and any uncommitted changes
2. Identify unpushed commits
3. Push changes to the remote repository
4. Analyze all commits since branching from the base branch
5. Generate a comprehensive PR message with summary, changes, and test plan
