"use client"

import { useState } from "react"
import { z } from "zod"
import { toast } from "sonner"
import { useForm } from "@tanstack/react-form"
import { useDropzone } from "react-dropzone"
import {
  AudioLines,
  FolderOpen,
  X,
  FileAudio,
  Upload,
  Mic,
  Tag,
  Play,
  Pause,
  Check,
  ChevronsUpDown,
  Globe,
  Layers,
  AlignLeft,
  Zap,
} from "lucide-react"
import locales from "locale-codes"

import { cn, formatFileSize } from "@/lib/utils"
import { useAudioPlayback } from "@/hooks/use-audio-playback"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Field, FieldError } from "@/components/ui/field"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command"
import {
  VOICE_CATEGORIES,
  VOICE_CATEGORY_LABELS,
  VOICE_VARIANTS,
  VOICE_VARIANT_LABELS,
} from "@/modules/voices/data/voice-categories"
import { useCreateVoice } from "@/modules/voices/hooks/use-create-voice"
import { VoiceRecorder } from "./voice-recorder"

const LANGUAGE_OPTIONS = locales.all
  .filter((l) => l.tag && l.tag.includes("-") && l.name)
  .map((l) => ({
    value: l.tag,
    label: l.location ? `${l.name} (${l.location})` : l.name,
  }))

const voiceCreateFormSchema = z.object({
  name: z.string().min(1, "Name is required"),
  file: z
    .instanceof(File, { message: "An audio file is required" })
    .nullable()
    .refine((f) => f !== null, "An audio file is required"),
  category: z.string().min(1, "A category is required"),
  language: z.string().min(1, "A language is required"),
  variant: z.string().min(1, "A voice type is required"),
  description: z.string(),
})

function FileDropzone({
  file,
  onFileChange,
  isInvalid,
}: {
  file: File | null
  onFileChange: (file: File | null) => void
  isInvalid?: boolean
}) {
  const { isPlaying, togglePlay } = useAudioPlayback(file)

  const { getRootProps, getInputProps, isDragActive, isDragReject } = useDropzone({
    accept: { "audio/*": [] },
    maxSize: 20 * 1024 * 1024,
    multiple: false,
    onDrop: (acceptedFiles) => {
      if (acceptedFiles.length > 0) {
        onFileChange(acceptedFiles[0])
      }
    },
  })

  if (file) {
    return (
      <div className="flex items-center gap-3 rounded-xl border p-4">
        <div className="flex size-10 items-center justify-center rounded-lg bg-muted">
          <FileAudio className="size-5 text-muted-foreground" />
        </div>

        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{file.name}</p>
          <p className="text-xs text-muted-foreground">{formatFileSize(file.size)}</p>
        </div>

        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={togglePlay}>
          {isPlaying ? <Pause className="size-4" /> : <Play className="size-4" />}
        </Button>

        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={() => onFileChange(null)}>
          <X className="size-4" />
        </Button>
      </div>
    )
  }

  return (
    <div
      {...getRootProps()}
      className={cn(
        "flex cursor-pointer flex-col items-center justify-center gap-4 overflow-hidden rounded-2xl border px-6 py-10 transition-colors",
        isDragReject || isInvalid
          ? "border-destructive"
          : isDragActive
            ? "border-primary"
            : "",
      )}>
      <input {...getInputProps()} />
      <div className="flex size-12 items-center justify-center rounded-xl bg-muted">
        <AudioLines className="size-5 text-muted-foreground" />
      </div>

      <div className="flex flex-col items-center gap-1.5">
        <p className="text-base font-semibold tracking-tight">Upload your audio file</p>
        <p className="text-center text-sm text-muted-foreground">
          Supports all audio formats, max size 20MB
        </p>
      </div>

      <Button
        type="button"
        variant="outline"
        size="sm">
        <FolderOpen className="size-3.5" />
        Upload file
      </Button>
    </div>
  )
}

function LanguageCombobox({
  value,
  onChange,
  isInvalid,
}: {
  value: string
  onChange: (value: string) => void
  isInvalid?: boolean
}) {
  const [open, setOpen] = useState(false)

  const selectedLabel = LANGUAGE_OPTIONS.find((l) => l.value === value)?.label ?? ""

  return (
    <Popover
      open={open}
      onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-invalid={isInvalid}
          className={cn(
            "h-9 w-full justify-between font-normal",
            !value && "text-muted-foreground",
          )}>
          <div className="flex min-w-0 items-center gap-2">
            <Globe className="size-4 shrink-0 text-muted-foreground" />
            <span className="truncate">
              {value ? selectedLabel : "Select language..."}
            </span>
          </div>
          <ChevronsUpDown className="size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) p-0">
        <Command>
          <CommandInput placeholder="Search language..." />
          <CommandList>
            <CommandEmpty>No language found.</CommandEmpty>
            <CommandGroup>
              {LANGUAGE_OPTIONS.map((lang) => (
                <CommandItem
                  key={lang.value}
                  value={lang.label}
                  onSelect={() => {
                    onChange(lang.value)
                    setOpen(false)
                  }}>
                  {lang.label}
                  <Check
                    className={cn(
                      "ml-auto size-4",
                      value === lang.value ? "opacity-100" : "opacity-0",
                    )}
                  />
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

interface VoiceCreateFormProps {
  scrollable?: boolean
  className?: string
  footer?: (submit: React.ReactNode) => React.ReactNode
  onError?: (message: string) => void
}

export function VoiceCreateForm({
  scrollable,
  className,
  footer,
  onError,
}: VoiceCreateFormProps) {
  const createMutation = useCreateVoice()

  const form = useForm({
    defaultValues: {
      name: "",
      file: null as File | null,
      category: "GENERAL" as string,
      variant: "NEUTRAL" as string,
      language: "en-US",
      description: "",
    },
    validators: {
      onSubmit: voiceCreateFormSchema,
    },
    onSubmit: async ({ value }) => {
      try {
        const audioData = new Uint8Array(await value.file!.arrayBuffer())

        await createMutation.mutateAsync({
          name: value.name,
          audioData,
          contentType: value.file?.type || "audio/wav",
          category: value.category,
          variant: value.variant,
          language: value.language,
          description: value.description || undefined,
        })

        toast.success("Voice created successfully!")
        form.reset()
      } catch (error) {
        const message = error instanceof Error ? error.message : "Failed to create voice"

        if (onError) {
          onError(message)
        } else {
          toast.error(message)
        }
      }
    },
  })

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault()
        form.handleSubmit()
      }}
      className={cn(
        "flex flex-col gap-6",
        scrollable && "min-h-0 flex-1 gap-0",
        className,
      )}>
      <div
        className={cn(
          "flex flex-col gap-6",
          scrollable && "no-scrollbar min-h-0 flex-1 overflow-y-auto pr-1",
        )}>
        <form.Field name="file">
          {(field) => {
            const isInvalid = field.state.meta.isTouched && !field.state.meta.isValid

            return (
              <Field data-invalid={isInvalid}>
                <Tabs defaultValue="upload">
                  <TabsList className="h-11! w-full">
                    <TabsTrigger value="upload">
                      <Upload className="size-3.5" />
                      Upload
                    </TabsTrigger>
                    <TabsTrigger value="record">
                      <Mic className="size-3.5" />
                      Record
                    </TabsTrigger>
                  </TabsList>

                  <TabsContent value="upload">
                    <FileDropzone
                      file={field.state.value}
                      onFileChange={field.handleChange}
                      isInvalid={isInvalid}
                    />
                  </TabsContent>

                  <TabsContent value="record">
                    <VoiceRecorder
                      file={field.state.value}
                      onFileChange={field.handleChange}
                      isInvalid={isInvalid}
                    />
                  </TabsContent>
                </Tabs>
                {isInvalid && <FieldError errors={field.state.meta.errors} />}
              </Field>
            )
          }}
        </form.Field>

        <form.Field name="name">
          {(field) => {
            const isInvalid = field.state.meta.isTouched && !field.state.meta.isValid

            return (
              <Field data-invalid={isInvalid}>
                <div className="relative flex items-center">
                  <div className="pointer-events-none absolute left-0 flex h-full w-11 items-center justify-center">
                    <Tag className="size-4 text-muted-foreground" />
                  </div>
                  <Input
                    id={field.name}
                    placeholder="Voice Label"
                    aria-invalid={isInvalid}
                    value={field.state.value}
                    onChange={(e) => field.handleChange(e.target.value)}
                    onBlur={field.handleBlur}
                    className="pl-10"
                  />
                </div>
                {isInvalid && <FieldError errors={field.state.meta.errors} />}
              </Field>
            )
          }}
        </form.Field>

        <form.Field name="category">
          {(field) => {
            const isInvalid = field.state.meta.isTouched && !field.state.meta.isValid

            return (
              <Field data-invalid={isInvalid}>
                <div className="relative flex items-center">
                  <div className="pointer-events-none absolute left-0 flex h-full w-11 items-center justify-center">
                    <Layers className="size-4 text-muted-foreground" />
                  </div>

                  <Select
                    value={field.state.value}
                    onValueChange={field.handleChange}>
                    <SelectTrigger className="w-full pl-10">
                      <SelectValue placeholder="Select category..." />
                    </SelectTrigger>
                    <SelectContent>
                      {VOICE_CATEGORIES.map((cat) => (
                        <SelectItem
                          key={cat}
                          value={cat}>
                          {VOICE_CATEGORY_LABELS[cat]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {isInvalid && <FieldError errors={field.state.meta.errors} />}
              </Field>
            )
          }}
        </form.Field>

        <form.Field name="variant">
          {(field) => {
            const isInvalid = field.state.meta.isTouched && !field.state.meta.isValid

            return (
              <Field data-invalid={isInvalid}>
                <div className="relative flex items-center">
                  <div className="pointer-events-none absolute left-0 flex h-full w-11 items-center justify-center">
                    <Zap className="size-4 text-muted-foreground" />
                  </div>

                  <Select
                    value={field.state.value}
                    onValueChange={field.handleChange}>
                    <SelectTrigger className="w-full pl-10">
                      <SelectValue placeholder="Select voice type..." />
                    </SelectTrigger>
                    <SelectContent>
                      {VOICE_VARIANTS.map((variant) => (
                        <SelectItem
                          key={variant}
                          value={variant}>
                          {VOICE_VARIANT_LABELS[variant]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                {isInvalid && <FieldError errors={field.state.meta.errors} />}
              </Field>
            )
          }}
        </form.Field>

        <form.Field name="language">
          {(field) => {
            const isInvalid = field.state.meta.isTouched && !field.state.meta.isValid

            return (
              <Field data-invalid={isInvalid}>
                <LanguageCombobox
                  value={field.state.value}
                  onChange={field.handleChange}
                  isInvalid={isInvalid}
                />
                {isInvalid && <FieldError errors={field.state.meta.errors} />}
              </Field>
            )
          }}
        </form.Field>

        <form.Field name="description">
          {(field) => {
            const isInvalid = field.state.meta.isTouched && !field.state.meta.isValid

            return (
              <Field data-invalid={isInvalid}>
                <div className="relative flex items-center">
                  <div className="pointer-events-none absolute left-0 top-0 flex h-11 w-11 items-center justify-center">
                    <AlignLeft className="size-4 text-muted-foreground" />
                  </div>
                  <Textarea
                    id={field.name}
                    placeholder="Describe this voice..."
                    aria-invalid={isInvalid}
                    value={field.state.value}
                    onChange={(e) => field.handleChange(e.target.value)}
                    onBlur={field.handleBlur}
                    className="min-h-20 resize-none pl-10"
                    rows={3}
                  />
                </div>
                {isInvalid && <FieldError errors={field.state.meta.errors} />}
              </Field>
            )
          }}
        </form.Field>
      </div>

      <form.Subscribe
        selector={(s) => ({
          isSubmitting: s.isSubmitting,
        })}>
        {({ isSubmitting }) => {
          const submitButton = (
            <Button
              type="submit"
              disabled={isSubmitting}>
              {isSubmitting ? "Creating..." : "Create Voice"}
            </Button>
          )

          return footer ? footer(submitButton) : submitButton
        }}
      </form.Subscribe>
    </form>
  )
}
