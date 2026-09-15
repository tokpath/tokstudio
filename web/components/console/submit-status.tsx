export function SubmitStatus({
  error,
  selectable,
}: {
  error?: string;
  selectable?: string;
}) {
  if (!error && !selectable) {
    return null;
  }
  return (
    <div className="space-y-2" role="alert" data-testid="submit-status">
      {error ? <p className="text-sm leading-relaxed text-danger">{error}</p> : null}
      {selectable ? (
        <code className="th-code block select-all overflow-x-auto whitespace-nowrap text-[13px] text-ink">
          {selectable}
        </code>
      ) : null}
    </div>
  );
}
