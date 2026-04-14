"use client"

import type { PlainMessage } from "@bufbuild/protobuf"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { getGenerationPlaybackUrlAction } from "@/actions/generations"
import { fetchAudioWithCache } from "@/lib/audio-fetch"

import { useAuth } from "@/lib/auth-context"
import { generationRpcClient } from "@/lib/rpc"
import {
  GenerateSpeechRequest,
  GenerationItem,
  GenerationJobStatus,
  GetGenerationRequest,
  GetGenerationResponse,
  ListGenerationsRequest,
} from "@/gen/pb/generation_pb"

export type GenerationItemType = PlainMessage<GenerationItem>
export type GenerateSpeechInput = PlainMessage<GenerateSpeechRequest>
export type GenerationDetailType = PlainMessage<GetGenerationResponse>

export function useGenerations() {
  const { accessToken } = useAuth()

  return useQuery({
    queryKey: ["generations", !!accessToken],
    enabled: Boolean(accessToken),
    staleTime: 30 * 1000,
    queryFn: async (): Promise<GenerationItemType[]> => {
      if (!accessToken) return []

      const response = await generationRpcClient.listGenerations(
        new ListGenerationsRequest(),
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
          },
        },
      )

      return response.generations as GenerationItemType[]
    },
  })
}

export function useGeneration(generationId: string) {
  const { accessToken } = useAuth()

  return useQuery({
    queryKey: ["generation", generationId, !!accessToken],
    enabled: Boolean(accessToken && generationId),
    staleTime: 30 * 1000,
    refetchInterval: (query) => {
      const status = (query.state.data as GenerationDetailType | undefined)?.status
      if (
        status === GenerationJobStatus.QUEUED ||
        status === GenerationJobStatus.PROCESSING
      ) {
        return 2000
      }

      return false
    },
    queryFn: async (): Promise<GenerationDetailType> => {
      if (!accessToken) throw new Error("Unauthorized")

      const response = await generationRpcClient.getGeneration(
        new GetGenerationRequest({ id: generationId }),
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
          },
        },
      )

      return response as GenerationDetailType
    },
  })
}

export function useGenerateSpeechMutation() {
  const { accessToken } = useAuth()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (input: GenerateSpeechInput) => {
      if (!accessToken) {
        throw new Error("Unauthorized")
      }

      return generationRpcClient.generateSpeech(input, {
        headers: {
          Authorization: `Bearer ${accessToken}`,
        },
      })
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["generations"] })
    },
  })
}

export function useGenerationPlaybackUrl(s3ObjectKey?: string) {
  const normalizedKey = s3ObjectKey?.trim() ?? ""

  return useQuery({
    queryKey: ["generation-playback-url", normalizedKey],
    enabled: Boolean(normalizedKey),
    staleTime: 55 * 60 * 1000,
    queryFn: async (): Promise<string> => {
      if (!normalizedKey) {
        throw new Error("s3ObjectKey is required")
      }

      if (/^https?:\/\//i.test(normalizedKey)) {
        return normalizedKey
      }

      const result = await getGenerationPlaybackUrlAction(normalizedKey)
      return result.url
    },
  })
}

/**
 * Hook for fetching audio with caching support
 * Returns a blob URL that can be used with audio elements
 * Automatically caches audio blobs to reduce S3 roundtrips
 */
export function useGenerationPlaybackUrlWithCache(s3ObjectKey?: string) {
  const normalizedKey = s3ObjectKey?.trim() ?? ""
  const playbackUrlQuery = useGenerationPlaybackUrl(normalizedKey)

  return useQuery({
    queryKey: ["generation-playback-url-cached", normalizedKey],
    enabled: Boolean(normalizedKey && playbackUrlQuery.data),
    staleTime: 55 * 60 * 1000,
    queryFn: async (): Promise<string> => {
      if (!normalizedKey || !playbackUrlQuery.data) {
        throw new Error("s3ObjectKey and playback URL are required")
      }

      return fetchAudioWithCache({
        url: playbackUrlQuery.data,
        key: normalizedKey,
      })
    },
  })
}
