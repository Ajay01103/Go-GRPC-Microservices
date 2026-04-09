import { DashboardView } from "@/modules/dashboard/views/dashboard-view"
import { requireAuthenticated } from "@/lib/server-auth"

const Dashboard = async () => {
  await requireAuthenticated()
  return <DashboardView />
}

export default Dashboard
