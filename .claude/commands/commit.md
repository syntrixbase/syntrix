# Git Commit Command

Create a well-structured git commit for the current changes.

## Instructions

1. **Analyze Changes**: Run `git status` and `git diff --staged` (or `git diff` if nothing staged) to understand what changed.

2. **Review Recent Commits**: Run `git log --oneline -10` to understand the commit message style used in this repository.

3. **Stage Changes** (if needed): If there are unstaged changes that should be committed, stage them with `git add`.

4. **Create New Branch** (if needed): If currently on `main`, create a new branch with a name based on changes with the prefix of `feats/`, `bugfix/` or any applicable prefix.

5. **Commit**: Create a commit message with this format and use HEREDOC:
   ```bash
   git commit -m "$(cat <<'EOF'
   <Summary: 50 chars or less, imperative mood>

   Changes:
   - <What was modified/added/deleted>
   - <What functionality was added/changed/fixed>

   <Why this change was made (if not obvious)>
   EOF
   )"
   ```

## Example

```bash
git commit -m "$(cat <<'EOF'
Fix rebuild startKey to use CurrentPosition from EventReplayer

Changes:
- Add CurrentPosition(ctx) method to EventReplayer interface
- Update doRebuild() to call CurrentPosition before storage scan
- Add tests for correct startKey usage and error handling

Previously startKey was hardcoded to empty string, causing replay
to start from buffer beginning instead of the recorded position.
EOF
)"
```

## Rules

- Do NOT add "Generated with Claude Code", "via Happy", or any co-author credits
- Do NOT push to remote unless explicitly asked
- Do NOT use `git commit --amend` unless explicitly asked
- Do NOT skip pre-commit hooks
- If commit fails due to pre-commit hook, fix the issue and create a NEW commit

## User Input

$ARGUMENTS
