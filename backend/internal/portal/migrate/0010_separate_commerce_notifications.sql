DELETE FROM portal_schedules WHERE kind='commerce_events';
DELETE FROM durable_jobs WHERE queue='portal-maintenance' AND kind='commerce_events';
