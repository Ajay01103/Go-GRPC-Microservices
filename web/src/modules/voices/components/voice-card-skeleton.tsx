import { Skeleton } from "@/components/ui/skeleton"

export function VoiceCardSkeleton() {
  return (
    <article className="flex flex-col gap-4 overflow-hidden rounded-xl border bg-card p-4 shadow-sm lg:flex-row lg:items-center lg:p-5">
      <div className="flex items-center gap-4 min-w-0 flex-1">
        {/* Avatar skeleton */}
        <div className="relative shrink-0">
          <Skeleton className="size-16 rounded-2xl" />
        </div>

        <div className="min-w-0 flex-1 space-y-2 w-full">
          {/* Title skeleton */}
          <Skeleton className="h-5 w-3/4" />

          {/* Badges skeleton */}
          <div className="flex flex-wrap items-center gap-2">
            <Skeleton className="h-6 w-20 rounded-full" />
            <Skeleton className="h-6 w-20 rounded-full" />
          </div>

          {/* Description skeleton */}
          <div className="space-y-1">
            <Skeleton className="h-4 w-full" />
            <Skeleton className="h-4 w-5/6" />
          </div>

          {/* Language/region skeleton */}
          <Skeleton className="h-6 w-24 rounded-full" />
        </div>
      </div>

      {/* Action button skeleton */}
      <div className="flex items-center justify-end gap-2">
        <Skeleton className="h-10 w-10 rounded-lg lg:hidden" />
        <Skeleton className="h-10 w-32 rounded-lg hidden lg:block" />
      </div>
    </article>
  )
}
