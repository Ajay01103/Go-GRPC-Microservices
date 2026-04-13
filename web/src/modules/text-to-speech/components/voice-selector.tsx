"use client"

import { useStore } from "@tanstack/react-form"

import { Field, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { VoiceAvatar } from "@/components/voice-avatar"
import { useTypedAppFormContext } from "@/hooks/use-app-form"

import { VoiceCategory } from "@/gen/pb/voice_pb"
import { getProtoVoiceCategoryLabel } from "@/modules/voices/data/voice-categories"

import { useTTSVoices } from "../contexts/tts-voices-context"
import { ttsFormOptions } from "./text-to-speech-form"

export function VoiceSelector() {
  const { customVoices, systemVoices, allVoices: voices } = useTTSVoices()

  const form = useTypedAppFormContext(ttsFormOptions)
  const voiceId = useStore(form.store, (s) => s.values.voiceId)
  const isSubmitting = useStore(form.store, (s) => s.isSubmitting)

  const selectedVoice = voices.find((v) => v.id === voiceId)
  const hasMissingSelectedVoice = Boolean(voiceId) && !selectedVoice
  const currentVoice = selectedVoice
    ? selectedVoice
    : hasMissingSelectedVoice
      ? {
          id: voiceId,
          name: "Unavailable voice",
          category: VoiceCategory.UNSPECIFIED,
        }
      : voices[0]

  const currentVoiceCategoryLabel = currentVoice
    ? getProtoVoiceCategoryLabel(currentVoice.category)
    : ""

  return (
    <Field>
      <FieldLabel>Voice style</FieldLabel>
      <Select
        value={voiceId}
        onValueChange={(v) => form.setFieldValue("voiceId", v)}
        disabled={isSubmitting}>
        <SelectTrigger className="w-full h-auto gap-1 rounded-lg bg-white px-2 py-1">
          <SelectValue>
            {currentVoice && (
              <>
                <VoiceAvatar
                  seed={currentVoice.id}
                  name={currentVoice.name}
                />
                <span className="truncate text-sm font-medium tracking-tight">
                  {currentVoice.name}
                  {currentVoice.category !== VoiceCategory.UNSPECIFIED &&
                    ` - ${currentVoiceCategoryLabel}`}
                </span>
              </>
            )}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {hasMissingSelectedVoice && currentVoice && (
            <>
              <SelectGroup>
                <SelectLabel>Selected Voice</SelectLabel>
                <SelectItem value={currentVoice.id}>
                  <VoiceAvatar
                    seed={currentVoice.id}
                    name={currentVoice.name}
                  />
                  <span className="truncate text-sm font-medium">
                    {currentVoice.name}
                    {currentVoice.category !== VoiceCategory.UNSPECIFIED &&
                      ` - ${currentVoiceCategoryLabel}`}
                  </span>
                </SelectItem>
              </SelectGroup>
              {(customVoices.length > 0 || systemVoices.length > 0) && <SelectSeparator />}
            </>
          )}
          {customVoices.length > 0 && (
            <SelectGroup>
              <SelectLabel>Team Voices</SelectLabel>
              {customVoices.map((v) => (
                <SelectItem
                  key={v.id}
                  value={v.id}>
                  <VoiceAvatar
                    seed={v.id}
                    name={v.name}
                  />
                  <span className="truncate text-sm font-medium">
                    {v.name} - {getProtoVoiceCategoryLabel(v.category)}
                  </span>
                </SelectItem>
              ))}
            </SelectGroup>
          )}
          {customVoices.length > 0 && systemVoices.length > 0 && <SelectSeparator />}
          {systemVoices.length > 0 && (
            <SelectGroup>
              <SelectLabel>Built-in Voices</SelectLabel>
              {systemVoices.map((v) => (
                <SelectItem
                  key={v.id}
                  value={v.id}>
                  <VoiceAvatar
                    seed={v.id}
                    name={v.name}
                  />
                  <span className="truncate text-sm font-medium">
                    {v.name} - {getProtoVoiceCategoryLabel(v.category)}
                  </span>
                </SelectItem>
              ))}
            </SelectGroup>
          )}
        </SelectContent>
      </Select>
    </Field>
  )
}
