import LoginForm from "@/modules/auth/components/login-form"
import { requireGuest } from "@/lib/server-auth"

const Page = async () => {
  await requireGuest()
  return <LoginForm />
}

export default Page
