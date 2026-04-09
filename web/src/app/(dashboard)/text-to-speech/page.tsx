import { TextToSpeechView } from "@/modules/text-to-speech/views/text-to-speech-view"
import { Metadata } from "next"

export const metadata: Metadata = { title: "Text to Speech" }

const TTSPage = () => {
  return <TextToSpeechView />
}

export default TTSPage
