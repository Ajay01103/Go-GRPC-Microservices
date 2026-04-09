import { VoicesView } from "@/modules/voices/views/voices-view"
import { Metadata } from "next"

export const metadata: Metadata = { title: "Voices" }

const VoicesPage = () => {
  return <VoicesView />
}

export default VoicesPage
