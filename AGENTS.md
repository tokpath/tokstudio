# Agent 约定

业务规则以 `docs/product-spec/TokenHub产品说明书.md` 及其配套案例、决策清单为准；工程实施以同目录的重构实施方案和实施工作包为准。`docs/业务说明.md` 仅保留入口，不再定义旧业务。用户本次明确指令优先于文档内的默认流程。

## 本轮已确认的实施约定（2026-10-16）

- 用户已确认产品说明书及重构方案，完整 W00—W09 均在交付范围内；文档中的评审状态为编制时快照，不能据此重复请求确认或缩减范围。
- 本轮主代理已 fetch 并核对 `HEAD...origin/release/v0.1.0` 为 `0 0`，基线提交为 `9f0d43fdff8d47ecb00f60b3b105daf9fafeae44`。
- 用户指定直接在 `release/v0.1.0` 上实施，覆盖方案中建立独立重构分支的默认条款。本轮不新建分支或 worktree；未经新的授权不提交、不推送、不发布、不清除运行环境。
- 用户在本轮本地验收完成后明确授权“提交推送”，主代理可将本轮重构提交并推送到 `origin/release/v0.1.0`；该授权不包含部署或清除运行环境。
- 共享 checkout 的协作代理只修改分配范围，不自行 fetch/merge/rebase/checkout/reset/clean/stash/commit/push。由主代理统一处理 Git 操作，避免覆盖其他代理正在进行的工作。
- 旧业务与数据没有兼容要求；新业务必须使用隔离新库。普通 release push 只运行 CI，发布通过人工 workflow_dispatch 选择完整提交 SHA 并验证 CI。

## 改代码之前必须先更新当前分支

**在做任何修改（改文件、提交、再开实现）之前，必须先把当前分支同步到远程最新基线。** 不要在过期的本地 checkout 上开工。

默认基线是 `origin/release/v0.1.0`（不是 `main`）。本轮已由主代理执行上述同步，共享协作代理无需重复执行。后续独立任务的步骤：

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
