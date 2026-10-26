-- Route failure protection has one system policy. Preserve in-flight lease snapshots.
UPDATE tokenhub_upstreams SET definition = definition - 'routing' WHERE definition ? 'routing';
