#!/usr/bin/env python3
"""Deep-merge leftover i18n keys into zh/en/ja catalogs."""
from __future__ import annotations

import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1] / "messages"

PATCH = {
    "catalog": {
        "zh": {"perSec": "/秒", "perImage": "/张"},
        "en": {"perSec": "/s", "perImage": "/img"},
        "ja": {"perSec": "/秒", "perImage": "/枚"},
    },
    "dashboard": {
        "zh": {
            "opsLead": "告警和应急手册在 /admin/alerts 与 /admin/runbooks。时间序列来自网关与账务接口，不直连业务表。",
            "refreshMetrics": "刷新指标",
            "exportDaily": "导出日报 CSV",
            "modelReqs": "模型请求",
            "last7d": "近 7 日",
        },
        "en": {
            "opsLead": "Alerts and runbooks live at /admin/alerts and /admin/runbooks. Series come from gateway and ledger APIs, not raw tables.",
            "refreshMetrics": "Refresh metrics",
            "exportDaily": "Export daily CSV",
            "modelReqs": "Model requests",
            "last7d": "Last 7 days",
        },
        "ja": {
            "opsLead": "告警とランブックは /admin/alerts と /admin/runbooks。時系列はゲートウェイと台帳 API からで、業務表には直結しません。",
            "refreshMetrics": "指標を更新",
            "exportDaily": "日次 CSV を書き出す",
            "modelReqs": "モデルリクエスト",
            "last7d": "直近 7 日",
        },
    },
    "overview": {
        "zh": {"hasReceipt": "有回单"},
        "en": {"hasReceipt": "Has receipt"},
        "ja": {"hasReceipt": "回単あり"},
    },
    "login": {
        "zh": {
            "emailInvalid": "请填写有效邮箱",
            "passMin": "密码至少 8 位",
            "registered": "已注册，渠道 {channel}",
            "fail": "失败",
            "welcome": "欢迎 {email}",
            "googleUnavailable": "Google 登录不可用",
            "googleRedirecting": "正在跳转 Google…",
            "googleUnconfigured": "未配置 Google 登录",
            "googleFinishing": "正在完成 Google 登录…",
            "googleDevHint": "开发环境：填写 Google 邮箱后继续（无需真实 OAuth）",
            "googleNoRedirect": "Google 登录未返回跳转地址",
            "googleNeedEmail": "请填写 Google 邮箱",
            "googleFail": "Google 登录失败",
            "promoPh": "可选 · THA1 / THB1 / THC1",
        },
        "en": {
            "emailInvalid": "Enter a valid email",
            "passMin": "Password must be at least 8 characters",
            "registered": "Signed up, channel {channel}",
            "fail": "Failed",
            "welcome": "Welcome {email}",
            "googleUnavailable": "Google sign-in is unavailable",
            "googleRedirecting": "Redirecting to Google…",
            "googleUnconfigured": "Google sign-in is not configured",
            "googleFinishing": "Finishing Google sign-in…",
            "googleDevHint": "Dev: enter a Google email to continue (no real OAuth)",
            "googleNoRedirect": "Google sign-in did not return a redirect",
            "googleNeedEmail": "Enter a Google email",
            "googleFail": "Google sign-in failed",
            "promoPh": "Optional · THA1 / THB1 / THC1",
        },
        "ja": {
            "emailInvalid": "有効なメールを入力してください",
            "passMin": "パスワードは 8 文字以上",
            "registered": "登録済み、チャネル {channel}",
            "fail": "失敗",
            "welcome": "ようこそ {email}",
            "googleUnavailable": "Google ログインは使えません",
            "googleRedirecting": "Google に移動しています…",
            "googleUnconfigured": "Google ログインは未設定です",
            "googleFinishing": "Google ログインを完了しています…",
            "googleDevHint": "開発環境：Google メールを入れて続行（本物の OAuth は不要）",
            "googleNoRedirect": "Google ログインのリダイレクトがありません",
            "googleNeedEmail": "Google メールを入力してください",
            "googleFail": "Google ログインに失敗",
            "promoPh": "任意 · THA1 / THB1 / THC1",
        },
    },
    "storefront": {
        "zh": {
            "loginFirst": "请先登录",
            "loginToBuy": "未登录，请先点「去登录」再回来购买",
            "needCode": "请填写兑换码",
            "redeemOk": "兑换成功 {amount} micro-USD",
            "loginToTopup": "请先登录再充值",
            "topupOk": "已创建充值单 {id}",
            "ordered": "已下单 {id}",
            "loginToSubscribe": "请先登录再订阅",
        },
        "en": {
            "loginFirst": "Sign in first",
            "loginToBuy": "Not signed in. Use Sign in, then come back to buy.",
            "needCode": "Enter a redeem code",
            "redeemOk": "Redeemed {amount} micro-USD",
            "loginToTopup": "Sign in to top up",
            "topupOk": "Created top-up {id}",
            "ordered": "Ordered {id}",
            "loginToSubscribe": "Sign in to subscribe",
        },
        "ja": {
            "loginFirst": "先にログインしてください",
            "loginToBuy": "未ログインです。「ログインへ」から戻って購入してください。",
            "needCode": "交換コードを入力してください",
            "redeemOk": "交換成功 {amount} micro-USD",
            "loginToTopup": "チャージにはログインが必要です",
            "topupOk": "チャージ {id} を作成",
            "ordered": "注文 {id}",
            "loginToSubscribe": "購読にはログインが必要です",
        },
    },
    "partnerBoard": {
        "zh": {
            "hint": "代理商看整棵树，1 级 KOL 看自己和 2 级，2 级只看直接引流。邮箱已脱敏。",
            "notPartner": "不是推广主体",
            "summary": "层级 {role} · 用户 {users} · 佣金 {comms} · 结算 {settlements}",
            "currentLine": "当前 {role} · 渠道 {channel} · {scope}",
            "notLoggedIn": "未登录",
            "seesDownline": "可看下级汇总",
            "seesDirect": "只看直接引流",
            "emptyUsersDetail": "登录推广主体后刷新，邮箱已脱敏。",
            "colEmail": "邮箱",
            "colCode": "推广码",
            "colStatus": "状态",
            "colKind": "类型",
            "colAmount": "金额",
            "emptyComms": "暂无佣金",
            "emptyCommsDetail": "冻结期满前不会出现可结算金额。",
            "emptySettle": "暂无结算单",
            "emptySettleDetail": "平台财务打款后才会出现在这里。",
            "colSettle": "结算单",
        },
        "en": {
            "hint": "Agents see the tree. Tier-1 KOLs see themselves and tier-2. Tier-2 sees direct referrals only. Emails are masked.",
            "notPartner": "Not a promotion principal",
            "summary": "Tier {role} · users {users} · commissions {comms} · settlements {settlements}",
            "currentLine": "Now {role} · channel {channel} · {scope}",
            "notLoggedIn": "Not signed in",
            "seesDownline": "Can see downline totals",
            "seesDirect": "Direct referrals only",
            "emptyUsersDetail": "Sign in as a promotion principal and refresh. Emails are masked.",
            "colEmail": "Email",
            "colCode": "Promo code",
            "colStatus": "Status",
            "colKind": "Kind",
            "colAmount": "Amount",
            "emptyComms": "No commissions",
            "emptyCommsDetail": "Nothing settleable until the freeze ends.",
            "emptySettle": "No settlements",
            "emptySettleDetail": "Rows appear after platform finance pays out.",
            "colSettle": "Settlement",
        },
        "ja": {
            "hint": "代理は木全体。1 級 KOL は自分と 2 級。2 級は直接紹介のみ。メールはマスク済み。",
            "notPartner": "紹介主体ではありません",
            "summary": "階層 {role} · ユーザー {users} · 手数料 {comms} · 精算 {settlements}",
            "currentLine": "現在 {role} · チャネル {channel} · {scope}",
            "notLoggedIn": "未ログイン",
            "seesDownline": "配下集計を見られる",
            "seesDirect": "直接紹介のみ",
            "emptyUsersDetail": "紹介主体でログインして更新。メールはマスク済み。",
            "colEmail": "メール",
            "colCode": "紹介コード",
            "colStatus": "状態",
            "colKind": "種類",
            "colAmount": "金額",
            "emptyComms": "手数料はありません",
            "emptyCommsDetail": "凍結が終わるまで精算可能額は出ません。",
            "emptySettle": "精算書はありません",
            "emptySettleDetail": "プラットフォーム財務の入金後に出ます。",
            "colSettle": "精算書",
        },
    },
}


def deep_merge(dst: dict, src: dict) -> dict:
    out = dict(dst)
    for k, v in src.items():
        if isinstance(v, dict) and isinstance(out.get(k), dict):
            out[k] = deep_merge(out[k], v)
        else:
            out[k] = v
    return out


def collect_keys(obj: object, prefix: str = "") -> set[str]:
    if not isinstance(obj, dict):
        return {prefix} if prefix else set()
    keys: set[str] = set()
    for k, v in obj.items():
        path = f"{prefix}.{k}" if prefix else str(k)
        if isinstance(v, dict):
            keys |= collect_keys(v, path)
        else:
            keys.add(path)
    return keys


def main() -> None:
    files = {loc: ROOT / f"{loc}.json" for loc in ("zh", "en", "ja")}
    data = {loc: json.loads(path.read_text()) for loc, path in files.items()}
    for ns, by_loc in PATCH.items():
        for loc, patch in by_loc.items():
            data[loc][ns] = deep_merge(data[loc].get(ns, {}), patch)
    zh_keys, en_keys, ja_keys = (collect_keys(data[loc]) for loc in ("zh", "en", "ja"))
    assert zh_keys == en_keys, sorted(zh_keys ^ en_keys)[:40]
    assert en_keys == ja_keys, sorted(en_keys ^ ja_keys)[:40]
    for loc, path in files.items():
        path.write_text(json.dumps(data[loc], ensure_ascii=False, indent=2) + "\n")
        print(f"wrote {path.name}")


if __name__ == "__main__":
    main()
