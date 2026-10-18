// Invoked only by the trusted release workflow, before exposing SSH secrets.
module.exports = async function deploymentTarget({ github, context, core }) {
  const branch = 'release/v0.1.0';
  const repository = `${context.repo.owner}/${context.repo.repo}`;
  const automatic = context.eventName === 'workflow_run';
  let sha;
  core.setOutput('deploy', 'false');
  if (automatic) {
    const run = context.payload.workflow_run;
    if (!run || run.event !== 'push' || run.head_branch !== branch ||
        run.head_repository?.full_name !== repository || run.conclusion !== 'success' || run.status !== 'completed') {
      throw new Error('Automatic deployment requires successful release push CI from this repository.');
    }
    sha = run.head_sha;
  } else if (context.eventName === 'workflow_dispatch') {
    if (context.ref !== `refs/heads/${branch}` || context.payload.inputs?.confirmation !== 'DEPLOY_TEST') {
      throw new Error('Manual deployment requires the release branch and DEPLOY_TEST.');
    }
    sha = context.payload.inputs.commit_sha;
  } else {
    throw new Error('Unsupported deployment event.');
  }
  if (!/^[0-9a-f]{40}$/.test(sha || '')) throw new Error('A complete commit SHA is required.');
  if (automatic) {
    const { data: current } = await github.rest.repos.getBranch({ ...context.repo, branch });
    if (current.commit.sha !== sha) {
      core.notice('A newer release commit exists; skipping this superseded deployment.');
      return;
    }
  }
  const { data } = await github.rest.actions.listWorkflowRuns({
    ...context.repo, workflow_id: 'ci.yml', branch, event: 'push', head_sha: sha, per_page: 100,
  });
  const runs = data.workflow_runs.filter(run => run.head_sha === sha && run.head_branch === branch && run.event === 'push');
  runs.sort((a, b) => b.run_number - a.run_number || b.run_attempt - a.run_attempt);
  if (!runs.length || runs[0].status !== 'completed' || runs[0].conclusion !== 'success') {
    throw new Error('Selected commit must have a completed successful CI run.');
  }
  core.setOutput('sha', sha);
  core.setOutput('deploy', 'true');
};
