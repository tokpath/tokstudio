import { CatalogFrame } from "@/components/catalog-frame";
import { EmptyState } from "@/components/empty-state";
import { Eyebrow } from "@/components/eyebrow";
import { RaisedCard } from "@/components/raised-card";
import type { NavItem } from "@/lib/nav";

function PageHeading({ item }: { item: NavItem }) {
  return (
    <div className="flex flex-col gap-2">
      <Eyebrow className="text-brand-emphasis">{item.eyebrow}</Eyebrow>
      <h1 className="text-2xl font-semibold">{item.title}</h1>
    </div>
  );
}

function UserOverview() {
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <RaisedCard eyebrow="Wallet" title="余额">
        接通钱包后这里用等宽数字显示可用余额。现在没有假金额。
      </RaisedCard>
      <RaisedCard eyebrow="Hold" title="预授权占用">
        预授权是扣住，不是已结算。状态会写成 HOLD，不会只用橙色圆点。
      </RaisedCard>
      <RaisedCard eyebrow="Keys" title="API Key">
        只显示名称和前缀。完整 Key 默认掩码，示例里只会写 sk-...xxxx。
      </RaisedCard>
      <RaisedCard eyebrow="Receipt" title="最近一张路由回单">
        public_model → provider → attempt N → 原因。客户只收一笔。
      </RaisedCard>
    </div>
  );
}

function ChannelHome({ item }: { item: NavItem }) {
  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 md:grid-cols-2">
        <RaisedCard eyebrow="Hold" title="佣金冻结">
          佣金按实际消耗冻结，退款会冲正。手机上先保证能看清这张表。
        </RaisedCard>
        <RaisedCard eyebrow="Quota" title="额度">
          额度按批发价累计消耗，不是充值金额。
        </RaisedCard>
      </div>
      {item.columns ? (
        <CatalogFrame columns={item.columns}>
          <EmptyState title={item.emptyTitle} detail={item.emptyDetail} />
        </CatalogFrame>
      ) : null}
    </div>
  );
}

function AdminOverview() {
  return (
    <div className="grid gap-4 md:grid-cols-2">
      <RaisedCard eyebrow="Reconcile" title="待对账">
        usage 缺失进入 HOLD / RECONCILE，不用灰字「未知」。没有数据时不画假曲线。
      </RaisedCard>
      <RaisedCard eyebrow="Margin" title="毛利">
        客户收入和上游成本分开看。毛利不用彩虹面积图。
      </RaisedCard>
      <RaisedCard eyebrow="Liability" title="佣金负债">
        冻结与可结算分列。调整佣金必须二次确认。
      </RaisedCard>
      <RaisedCard eyebrow="Health" title="Provider 健康">
        available / degraded / unavailable / maintenance 会带字标。
      </RaisedCard>
    </div>
  );
}

export function ConsolePage({ item }: { item: NavItem }) {
  return (
    <div className="mx-auto flex max-w-5xl flex-col gap-6">
      <PageHeading item={item} />
      {item.hero === "user-overview" ? <UserOverview /> : null}
      {item.hero === "channel-home" ? <ChannelHome item={item} /> : null}
      {item.hero === "admin-overview" ? <AdminOverview /> : null}
      {!item.hero && item.columns ? (
        <CatalogFrame columns={item.columns}>
          <EmptyState title={item.emptyTitle} detail={item.emptyDetail} />
        </CatalogFrame>
      ) : null}
      {!item.hero && !item.columns ? (
        <section className="rounded-stamp border border-hairline bg-canvas-raised">
          <EmptyState title={item.emptyTitle} detail={item.emptyDetail} />
        </section>
      ) : null}
    </div>
  );
}
