"use client"

import { TextToSpeechDetailView } from "@/modules/text-to-speech/views/text-to-speech-details-view"

export default function TextToSpeechDetailPage({
  params,
}: {
  params: Promise<{ generationId: string }>
}) {
  return <TextToSpeechDetailView params={params} />
}
