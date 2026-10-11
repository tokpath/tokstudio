-- 套餐名在同一归属内唯一：平台一份，每个 OEM 一份。
-- 不同归属可以同名。同一归属里只保留最新的一条，其余连同购买记录一并清除。

UPDATE plans_product_plans
SET name = btrim(name), updated_at = now()
WHERE name <> btrim(name);

DELETE FROM plans_entitlement_ledger
WHERE account_id IN (
    SELECT account.id
    FROM plans_entitlement_accounts AS account
    JOIN plans_subscriptions AS subscription
      ON account.source_type = 'plan' AND account.source_id = subscription.id
    WHERE subscription.plan_id IN (
        SELECT id FROM (
            SELECT id,
                   ROW_NUMBER() OVER (
                       PARTITION BY owner_type, owner_id, lower(name)
                       ORDER BY created_at DESC, id DESC
                   ) AS rn
            FROM plans_product_plans
        ) ranked
        WHERE rn > 1
    )
);

DELETE FROM plans_entitlement_accounts
WHERE source_type = 'plan'
  AND source_id IN (
      SELECT subscription.id
      FROM plans_subscriptions AS subscription
      WHERE subscription.plan_id IN (
          SELECT id FROM (
              SELECT id,
                     ROW_NUMBER() OVER (
                         PARTITION BY owner_type, owner_id, lower(name)
                         ORDER BY created_at DESC, id DESC
                     ) AS rn
              FROM plans_product_plans
          ) ranked
          WHERE rn > 1
      )
  );

DELETE FROM plans_subscriptions
WHERE plan_id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY owner_type, owner_id, lower(name)
                   ORDER BY created_at DESC, id DESC
               ) AS rn
        FROM plans_product_plans
    ) ranked
    WHERE rn > 1
);

DELETE FROM plans_plan_items
WHERE plan_id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY owner_type, owner_id, lower(name)
                   ORDER BY created_at DESC, id DESC
               ) AS rn
        FROM plans_product_plans
    ) ranked
    WHERE rn > 1
);

DELETE FROM plans_product_plans
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY owner_type, owner_id, lower(name)
                   ORDER BY created_at DESC, id DESC
               ) AS rn
        FROM plans_product_plans
    ) ranked
    WHERE rn > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS plans_product_plans_owner_name_uidx
    ON plans_product_plans (owner_type, owner_id, lower(name));
