"use client"

import { PauseIcon, PlayIcon } from "lucide-react"
import { useState } from "react"

import { getVoicePlaybackUrlAction } from "@/actions/voices"
import {
  AudioPlayerDuration,
  AudioPlayerProgress,
  AudioPlayerProvider,
  AudioPlayerTime,
  useAudioPlayer,
} from "@/components/ui/audio-player"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/lib/auth-context"

type VoiceAudioPreviewProps = {
  voiceId: string
}

type CachedPlayback = {
  url: string
  expiresAtUnix: number
}

const playbackUrlCache = new Map<string, CachedPlayback>()
const inFlightRequests = new Map<string, Promise<CachedPlayback>>()

const REFRESH_BUFFER_SECONDS = 20

async function getCachedPlaybackUrl(
  voiceId: string,
  accessToken: string | null,
): Promise<CachedPlayback> {
  const nowUnix = Math.floor(Date.now() / 1000)
  const cached = playbackUrlCache.get(voiceId)

  if (cached && cached.expiresAtUnix > nowUnix + REFRESH_BUFFER_SECONDS) {
    return cached
  }

  const existingRequest = inFlightRequests.get(voiceId)
  if (existingRequest) {
    return existingRequest
  }

  const request = getVoicePlaybackUrlAction(voiceId, accessToken)
    .then((result) => {
      playbackUrlCache.set(voiceId, result)
      return result
    })
    .finally(() => {
      inFlightRequests.delete(voiceId)
    })

  inFlightRequests.set(voiceId, request)
  return request
}

function VoiceAudioPreviewContent({ voiceId }: VoiceAudioPreviewProps) {
  const { accessToken } = useAuth()
  const player = useAudioPlayer()
  const [isLoadingUrl, setIsLoadingUrl] = useState(false)

  const isActive = player.isItemActive(voiceId)

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
      setIsLoadingUrl(true)
      const playback = await getCachedPlaybackUrl(voiceId, accessToken)

      await player.play({
        id: voiceId,
        src: playback.url,
      })
    } catch (error) {
      console.error("Failed to fetch playback url", error)
    } finally {
      setIsLoadingUrl(false)
    }
  }

  return (
    <div className="rounded-lg border bg-background p-2.5">
      <div className="flex items-center gap-2">
        <Button
          type="button"
          size="icon"
          variant="secondary"
          disabled={isLoadingUrl}
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

export function VoiceAudioPreview({ voiceId }: VoiceAudioPreviewProps) {
  return (
    <AudioPlayerProvider>
      <VoiceAudioPreviewContent voiceId={voiceId} />
    </AudioPlayerProvider>
  )
}
