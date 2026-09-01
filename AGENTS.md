# Agent 约定

给在本仓库改代码的人或 Agent 看。和产品文档冲突时，以 `docs/00`–`08` 与 `DESIGN.md` 为准；和本文件的流程约定冲突时，以本文件为准。

## 改代码之前必须先更新当前分支

**在做任何修改（改文件、提交、再开实现）之前，必须先把当前分支同步到远程最新基线。** 不要在过期的本地 checkout 上开工。

默认基线是 `origin/release/v0.1.0`（不是 `main`）。步骤：

1. `git fetch origin release/v0.1.0`
2. `git fetch origin <当前分支>`（分支已推过远程时）
3. 看和基线的前后差：`git rev-list --left-right --count HEAD...origin/release/v0.1.0`
4. 若落后基线：先 `git merge origin/release/v0.1.0`（或按任务要求 rebase），解决冲突后再改业务代码
5. 确认工作区干净、已经站在最新基线之上，再开始改文件

本机 `release/v0.1.0` 可能落后远程一到数小时，**不要只信本地分支名**，以 `origin/release/v0.1.0` 为准。

## 合并 PR

合入一律用 **Squash Merge**。不要用 merge commit，也不要用 rebase merge。

1. 无冲突：`gh pr merge <编号> --squash --delete-branch`。合入成功后必须删掉原 PR 分支（`--delete-branch` 会删远程；本地还在就 `git branch -d <分支>`）。
2. 有冲突：先在 PR 分支上同步基线、解决冲突并推送，再 squash merge。冲突未解决时不要合，也不要删分支。
3. 不要 force push 踢开冲突，也不要跳过冲突解决。
