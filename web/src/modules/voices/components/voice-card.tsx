import Link from "next/link"
import { BadgeInfo, Library, Mic, MoveRight } from "lucide-react"

import type { PlainMessage } from "@bufbuild/protobuf"

import { Button } from "@/components/ui/button"
import { VoiceAvatar } from "@/components/voice-avatar"
import { VoiceItem } from "@/gen/pb/voice_pb"
import {
  getProtoVoiceCategoryLabel,
  getProtoVoiceVariantLabel,
} from "@/modules/voices/data/voice-categories"
import { VoiceAudioPreview } from "./voice-audio-preview"

export type VoiceItemType = PlainMessage<VoiceItem>

interface VoiceCardProps {
  voice: VoiceItemType
}

const regionNames = new Intl.DisplayNames(["en"], { type: "region" })

function parseLanguage(locale: string) {
  const [language, country] = locale.split("-")
  if (!country) {
    return { language: locale, flag: "", region: locale.toUpperCase() }
  }

  const flag = [...country.toUpperCase()]
    .map((character) => String.fromCodePoint(0x1f1e6 + character.charCodeAt(0) - 65))
    .join("")

  return {
    language,
    flag,
    region: regionNames.of(country) ?? country,
  }
}

export function VoiceCard({ voice }: VoiceCardProps) {
  const { flag, region } = parseLanguage(voice.language || "")
  const categoryLabel = getProtoVoiceCategoryLabel(voice.category)
  const variantLabel = getProtoVoiceVariantLabel(voice.variant)

  return (
    <article className="group flex flex-col gap-4 overflow-hidden rounded-xl border bg-card p-4 shadow-sm transition-all hover:border-foreground/20 hover:shadow-md lg:flex-row lg:items-center lg:p-5">
      <div className="flex items-center gap-4 min-w-0 flex-1">
        <div className="relative shrink-0">
          <div className="absolute inset-0 rounded-2xl bg-linear-to-br from-muted/80 to-muted" />
          <VoiceAvatar
            seed={voice.id}
            name={voice.name}
            className="relative size-16 border-2 border-background shadow-sm"
          />
        </div>

        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="min-w-0 truncate text-base font-semibold tracking-tight text-foreground">
              {voice.name}
            </h4>
            <span className="inline-flex items-center gap-1 rounded-full bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground">
              <Library className="size-3.5" />
              {categoryLabel}
            </span>
            <span className="inline-flex items-center gap-1 rounded-full bg-muted px-2.5 py-1 text-xs font-medium text-muted-foreground">
              <BadgeInfo className="size-3.5" />
              {variantLabel}
            </span>
          </div>

          <p className="line-clamp-2 text-sm text-muted-foreground">
            {voice.description || "No description provided."}
          </p>

          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span className="inline-flex items-center gap-1 rounded-full border bg-background px-2.5 py-1 font-medium">
              <span>{flag}</span>
              <span>{region}</span>
            </span>
            {voice.language ? (
              <span className="inline-flex items-center gap-1 rounded-full border bg-background px-2.5 py-1 font-medium uppercase tracking-wide">
                {voice.language}
              </span>
            ) : null}
            <span className="inline-flex items-center gap-1 rounded-full border bg-background px-2.5 py-1 font-medium">
              <Mic className="size-3.5" />
              {voice.id}
            </span>
          </div>

          <VoiceAudioPreview voiceId={voice.id} />
        </div>
      </div>

      <div className="flex shrink-0 items-center justify-end gap-2 lg:pl-4">
        <Button
          asChild
          size="sm"
          variant="secondary">
          <Link href={`/text-to-speech?voiceId=${encodeURIComponent(voice.id)}`}>
            Use this voice
            <MoveRight className="size-4" />
          </Link>
        </Button>
      </div>
    </article>
  )
}
