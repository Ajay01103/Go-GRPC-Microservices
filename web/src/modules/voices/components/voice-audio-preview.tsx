"use client"

import { PauseIcon, PlayIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { cn } from "@/lib/utils"

import {
  AudioPlayerDuration,
  AudioPlayerProgress,
  AudioPlayerProvider,
  AudioPlayerTime,
  useAudioPlayer,
} from "@/components/ui/audio-player"
import { Button } from "@/components/ui/button"
import { fetchAudioWithCache, getCachedAudioObjectUrl } from "@/lib/audio-fetch"
import { getVoiceProviderKey, setVoiceProviderKey } from "@/lib/audio-cache"
import { getVoicePlayback } from "@/modules/voices/hooks/use-voice-playback"

type VoiceAudioPreviewProps = {
  voiceId: string
  src?: string
  itemId?: string
  className?: string
}

function VoiceAudioPreviewContent({
  voiceId,
  src,
  itemId,
  className,
}: VoiceAudioPreviewProps) {
  const queryClient = useQueryClient()
  const player = useAudioPlayer()
  const [isLoadingAudio, setIsLoadingAudio] = useState(false)
  const playerItemID = itemId?.trim() || voiceId
  const objectUrlRef = useRef<string | null>(null)

  const isActive = player.isItemActive(playerItemID)

  useEffect(() => {
    return () => {
      if (objectUrlRef.current) {
        URL.revokeObjectURL(objectUrlRef.current)
        objectUrlRef.current = null
      }
    }
  }, [playerItemID])

  async function handlePlayPause() {
    if (isActive) {
      if (player.isPlaying) {
        player.pause()
      } else {
        await player.play()
      }
      return
    }

    try {
      if (src) {
        await player.play({
          id: playerItemID,
          src,
        })
        return
      }

      setIsLoadingAudio(true)
      const persistedProviderKey = await getVoiceProviderKey(voiceId)
      if (persistedProviderKey) {
        const cachedOnlySrc = await getCachedAudioObjectUrl(persistedProviderKey)
        if (cachedOnlySrc) {
          if (objectUrlRef.current && objectUrlRef.current !== cachedOnlySrc) {
            URL.revokeObjectURL(objectUrlRef.current)
          }
          objectUrlRef.current = cachedOnlySrc

          await player.play({
            id: playerItemID,
            src: cachedOnlySrc,
          })
          return
        }
      }

      const playback = await getVoicePlayback(queryClient, voiceId)
      await setVoiceProviderKey(voiceId, playback.providerKey)
      const resolvedSrc = await fetchAudioWithCache({
        url: playback.url,
        key: playback.providerKey,
      })

      if (objectUrlRef.current && objectUrlRef.current !== resolvedSrc) {
        URL.revokeObjectURL(objectUrlRef.current)
      }
      objectUrlRef.current = resolvedSrc

      await player.play({
        id: playerItemID,
        src: resolvedSrc,
      })
    } catch (error) {
      console.error("Failed to fetch playback url", error)
    } finally {
      setIsLoadingAudio(false)
    }
  }

  return (
    <div className={cn("rounded-lg border bg-background p-2.5", className)}>
      <div className="flex items-center gap-2">
        <Button
          type="button"
          size="icon"
          variant="secondary"
          disabled={isLoadingAudio && !src}
          onClick={handlePlayPause}
          aria-label={isActive && player.isPlaying ? "Pause preview" : "Play preview"}>
          {isActive && player.isPlaying ? (
            <PauseIcon className="size-4" />
          ) : (
            <PlayIcon className="size-4" />
          )}
        </Button>

        <div className="min-w-0 flex-1">
          <AudioPlayerProgress className="w-full" />
          <div className="mt-1 flex items-center justify-between text-xs text-muted-foreground">
            <AudioPlayerTime className="text-xs" />
            <AudioPlayerDuration className="text-xs" />
          </div>
        </div>
      </div>
    </div>
  )
}

export function VoiceAudioPreview({
  voiceId,
  src,
  itemId,
  className,
}: VoiceAudioPreviewProps) {
  return (
    <AudioPlayerProvider>
      <VoiceAudioPreviewContent
        voiceId={voiceId}
        src={src}
        itemId={itemId}
        className={className}
      />
    </AudioPlayerProvider>
  )
}
