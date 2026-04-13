"use client"

import { useStore } from "@tanstack/react-form"

import { Button } from "@/components/ui/button"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Slider } from "@/components/ui/slider"
import { useTypedAppFormContext } from "@/hooks/use-app-form"

import { sliders } from "@/modules/text-to-speech/data/sliders"
import { ttsFormOptions } from "@/modules/text-to-speech/components/text-to-speech-form"
import { VoiceSelector } from "@/modules/text-to-speech/components/voice-selector"

export function SettingsPanelSettings() {
  const form = useTypedAppFormContext(ttsFormOptions)
  const isSubmitting = useStore(form.store, (s) => s.isSubmitting)

  const handleReset = () => {
    sliders.forEach((slider) => {
      form.setFieldValue(slider.id as any, slider.defaultValue)
    })
  }

  return (
    <>
      {/* Voice Style Dropdown Section */}
      <div className="border-b border-dashed p-4">
        <VoiceSelector />
      </div>

      {/* Voice Adjustments Section */}
      <div className="p-4 flex-1 flex flex-col">
        <FieldGroup className="gap-8 flex-1">
          {sliders.map((slider) => (
            <form.Field
              key={slider.id}
              name={slider.id}>
              {(field) => (
                <Field>
                  <FieldLabel>{slider.label}</FieldLabel>
                  <div className="flex items-center justify-between">
                    <span className="text-xs text-muted-foreground">{slider.leftLabel}</span>
                    <span className="text-xs text-muted-foreground">{slider.rightLabel}</span>
                  </div>
                  <Slider
                    value={[field.state.value]}
                    onValueChange={(value) => field.handleChange(value[0])}
                    min={slider.min}
                    max={slider.max}
                    step={slider.step}
                    disabled={isSubmitting}
                    className="**:data-[slot=slider-thumb]:size-3 **:data-[slot=slider-thumb]:bg-foreground **:data-[slot=slider-track]:h-1"
                  />
                </Field>
              )}
            </form.Field>
          ))}
        </FieldGroup>

        {/* Reset Button */}
        <Button
          type="button"
          variant="outline"
          onClick={handleReset}
          disabled={isSubmitting}
          className="mt-4 w-full">
          Reset to Defaults
        </Button>
      </div>
    </>
  )
}
