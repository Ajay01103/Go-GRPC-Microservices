import { BadgeInfo, Library, Loader2, MoreVertical, MoveRight, Pencil, Trash2 } from "lucide-react"
import Link from "next/link"
import { useState } from "react"
import { toast } from "sonner"

import type { PlainMessage } from "@bufbuild/protobuf"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import { VoiceAvatar } from "@/components/voice-avatar"
import { VoiceItem } from "@/gen/pb/voice_pb"
import {
  getProtoVoiceCategoryLabel,
  getProtoVoiceVariantLabel,
} from "@/modules/voices/data/voice-categories"
import { useDeleteVoice } from "@/modules/voices/hooks/use-delete-voice"
import { useUpdateVoice } from "@/modules/voices/hooks/use-update-voice"
import { VoiceAudioPreview } from "./voice-audio-preview"

export type VoiceItemType = PlainMessage<VoiceItem>

interface VoiceCardProps {
  voice: VoiceItemType
  isCustom?: boolean
}

const CATEGORY_OPTIONS = [
  { value: "GENERAL", label: "General" },
  { value: "NARRATION", label: "Narration" },
  { value: "CHARACTER", label: "Character" },
] as const

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

export function VoiceCard({ voice, isCustom = false }: VoiceCardProps) {
  const [isEditOpen, setIsEditOpen] = useState(false)
  const [isDeleteOpen, setIsDeleteOpen] = useState(false)
  const [name, setName] = useState(voice.name)
  const [description, setDescription] = useState(voice.description ?? "")
  const [language, setLanguage] = useState(voice.language)
  const [category, setCategory] = useState<"GENERAL" | "NARRATION" | "CHARACTER">("GENERAL")

  const updateVoice = useUpdateVoice()
  const deleteVoice = useDeleteVoice()

  const { flag, region } = parseLanguage(voice.language || "")
  const categoryLabel = getProtoVoiceCategoryLabel(voice.category)
  const variantLabel = getProtoVoiceVariantLabel(voice.variant)

  function openEditDialog() {
    setName(voice.name)
    setDescription(voice.description ?? "")
    setLanguage(voice.language)
    setCategory(voice.category === 2 ? "NARRATION" : voice.category === 3 ? "CHARACTER" : "GENERAL")
    setIsEditOpen(true)
  }

  async function handleUpdateVoice() {
    try {
      await updateVoice.mutateAsync({
        id: voice.id,
        name,
        description,
        language,
        category,
      })
      toast.success("Voice updated")
      setIsEditOpen(false)
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to update voice"
      toast.error(message)
    }
  }

  async function handleDeleteVoice() {
    try {
      await deleteVoice.mutateAsync({ id: voice.id })
      toast.success("Voice deleted")
      setIsDeleteOpen(false)
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to delete voice"
      toast.error(message)
    }
  }

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
            {/* <span className="inline-flex items-center gap-1 rounded-full border bg-background px-2.5 py-1 font-medium">
              <Mic className="size-3.5" />
              {voice.id}
            </span> */}
          </div>

          <VoiceAudioPreview voiceId={voice.id} />
        </div>
      </div>

      <div className="flex shrink-0 items-center justify-end gap-2 lg:pl-4">
        {isCustom ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Custom voice actions">
                <MoreVertical className="size-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              align="end"
              sideOffset={8}
              className="w-44 min-w-44">
              <DropdownMenuItem
                className="justify-start whitespace-nowrap"
                onClick={openEditDialog}>
                <Pencil className="size-4" />
                Edit voice
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                className="justify-start whitespace-nowrap"
                onClick={() => setIsDeleteOpen(true)}>
                <Trash2 className="size-4" />
                Delete voice
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}

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

      <Dialog
        open={isEditOpen}
        onOpenChange={setIsEditOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Edit custom voice</DialogTitle>
            <DialogDescription>Update this custom voice metadata.</DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">Name</label>
              <Input
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="Voice name"
              />
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium">Description</label>
              <Textarea
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder="Describe this voice"
                rows={3}
              />
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <label className="text-sm font-medium">Category</label>
                <Select
                  value={category}
                  onValueChange={(value) =>
                    setCategory(value as "GENERAL" | "NARRATION" | "CHARACTER")
                  }>
                  <SelectTrigger>
                    <SelectValue placeholder="Select category" />
                  </SelectTrigger>
                  <SelectContent>
                    {CATEGORY_OPTIONS.map((option) => (
                      <SelectItem
                        key={option.value}
                        value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-medium">Language</label>
                <Input
                  value={language}
                  onChange={(event) => setLanguage(event.target.value)}
                  placeholder="en-US"
                />
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setIsEditOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleUpdateVoice}
              disabled={updateVoice.isPending}>
              {updateVoice.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              Save changes
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog
        open={isDeleteOpen}
        onOpenChange={setIsDeleteOpen}>
        <AlertDialogContent size="sm">
          <AlertDialogHeader>
            <AlertDialogTitle>Delete custom voice?</AlertDialogTitle>
            <AlertDialogDescription>
              This will delete the voice and its uploaded audio from storage.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteVoice.isPending}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={handleDeleteVoice}
              disabled={deleteVoice.isPending}>
              {deleteVoice.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </article>
  )
}
