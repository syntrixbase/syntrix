---
description: Create a commit for all changes with an auto-generated message
---

Invoke the `commit-changes` agent using the Task tool to handle this commit.

Do NOT attempt to run git commands directly. Instead, launch the commit-changes agent which will:
1. Analyze all changes (staged, unstaged, and untracked files)
2. Stage all changes automatically
3. Generate a well-structured commit message
4. Create a new branch if currently on main/master
5. Execute the commit
