import { VoiceCategory, VoiceVariant } from "@/gen/pb/voice_pb"

const VOICE_CATEGORY_LABELS = {
  AUDIOBOOK: "Audiobook",
  CONVERSATIONAL: "Conversational",
  CUSTOMER_SERVICE: "Customer Service",
  GENERAL: "General",
  NARRATIVE: "Narrative",
  CHARACTERS: "Characters",
  MEDITATION: "Meditation",
  MOTIVATIONAL: "Motivational",
  PODCAST: "Podcast",
  ADVERTISING: "Advertising",
  VOICEOVER: "Voiceover",
  CORPORATE: "Corporate",
} as const

export type VoiceCategoryName = keyof typeof VOICE_CATEGORY_LABELS

const VOICE_VARIANT_LABELS = {
  MALE: "Male",
  FEMALE: "Female",
  NEUTRAL: "Neutral",
} as const

export type VoiceVariantName = keyof typeof VOICE_VARIANT_LABELS

const VOICE_CATEGORIES = Object.keys(VOICE_CATEGORY_LABELS) as VoiceCategoryName[]
const VOICE_VARIANTS = Object.keys(VOICE_VARIANT_LABELS) as VoiceVariantName[]

const PROTO_VOICE_CATEGORY_LABELS: Record<VoiceCategory, string> = {
  [VoiceCategory.UNSPECIFIED]: "Unspecified",
  [VoiceCategory.GENERAL]: "General",
  [VoiceCategory.NARRATION]: "Narration",
  [VoiceCategory.CHARACTER]: "Character",
}

const PROTO_VOICE_VARIANT_LABELS: Record<VoiceVariant, string> = {
  [VoiceVariant.UNSPECIFIED]: "Unspecified",
  [VoiceVariant.MALE]: "Male",
  [VoiceVariant.FEMALE]: "Female",
  [VoiceVariant.NEUTRAL]: "Neutral",
}

export function getVoiceCategoryLabel(category: VoiceCategoryName | undefined) {
  return VOICE_CATEGORY_LABELS[category ?? "GENERAL"]
}

export function getVoiceVariantLabel(variant: VoiceVariantName | undefined) {
  return VOICE_VARIANT_LABELS[variant ?? "NEUTRAL"]
}

export function getProtoVoiceCategoryLabel(category: VoiceCategory | undefined) {
  return PROTO_VOICE_CATEGORY_LABELS[category ?? VoiceCategory.UNSPECIFIED]
}

export function getProtoVoiceVariantLabel(variant: VoiceVariant | undefined) {
  return PROTO_VOICE_VARIANT_LABELS[variant ?? VoiceVariant.UNSPECIFIED]
}

export { VOICE_CATEGORY_LABELS, VOICE_VARIANT_LABELS, VOICE_CATEGORIES, VOICE_VARIANTS }
