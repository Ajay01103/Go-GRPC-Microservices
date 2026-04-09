import RegisterForm from "@/modules/auth/components/register-form"
import { requireGuest } from "@/lib/server-auth"

const Page = async () => {
  await requireGuest()
  return <RegisterForm />
}

export default Page
