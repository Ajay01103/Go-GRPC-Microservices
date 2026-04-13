import { AudioLines, Mic, Volume2 } from "lucide-react"

import { VoiceCard, VoiceItemType } from "./voice-card"
import { VoiceCardSkeleton } from "./voice-card-skeleton"

interface VoicesListProps {
  title: string
  voices: VoiceItemType[]
  isLoading?: boolean
  isCustom?: boolean
}

export function VoicesList({
  title,
  voices,
  isLoading = false,
  isCustom = false,
}: VoicesListProps) {
  // Show skeletons while loading
  if (isLoading) {
    return (
      <div className="space-y-4">
        <h3 className="text-lg font-semibold tracking-tight">{title}</h3>
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
          {Array.from({ length: 2 }).map((_, index) => (
            <VoiceCardSkeleton key={index} />
          ))}
        </div>
      </div>
    )
  }

  // Show empty state only when not loading and no voices
  if (!voices.length) {
    return (
      <div className="space-y-4">
        <h3 className="text-lg font-semibold tracking-tight">{title}</h3>

        <div className="flex flex-col items-center justify-center gap-3 py-12">
          <div className="relative flex h-14 w-32 items-center justify-center">
            <div className="absolute left-0 -rotate-30 rounded-full bg-muted p-4">
              <Volume2 className="size-5 text-muted-foreground" />
            </div>

            <div className="relative z-10 rounded-full bg-foreground p-4">
              <Mic className="size-5 text-background" />
            </div>

            <div className="absolute right-0 rotate-30 rounded-full bg-muted p-4">
              <AudioLines className="size-5 text-muted-foreground" />
            </div>
          </div>

          <p className="text-lg font-semibold tracking-tight text-foreground">No voices found</p>

          <p className="max-w-md text-center text-sm text-muted-foreground">
            {title} will appear here
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <h3 className="text-lg font-semibold tracking-tight">{title}</h3>
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {voices.map((voice) => (
          <VoiceCard
            key={voice.id}
            voice={voice}
            isCustom={isCustom}
          />
        ))}
      </div>
    </div>
  )
}
