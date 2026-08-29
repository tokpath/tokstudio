-- 用户控制台个人设置：显示名与界面语言。渠道归属仍不可自助修改。
ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS locale TEXT NOT NULL DEFAULT 'zh';
