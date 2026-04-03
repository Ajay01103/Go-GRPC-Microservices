import { PageHeader } from "@/components/page-header";
import { HeroPattern } from "@/modules/dashboard/components/hero-pattern";
import { DashboardHeader } from "@/modules/dashboard/components/dashboard-header";
import { TextInputPanel } from "@/modules/dashboard/components/text-input-panel";
import { QuickActionsPanel } from "@/modules/dashboard/components/quick-actions-panel";

export function DashboardView() {
  return (
    <div className="relative">
      <PageHeader title="Dashboard" className="lg:hidden" />
      <HeroPattern />
      <div className="relative space-y-8 p-4 lg:p-16">
        <DashboardHeader />
        <TextInputPanel />
        <QuickActionsPanel />
      </div>
    </div>
  );
}
