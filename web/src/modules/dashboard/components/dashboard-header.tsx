"use client"

import { Headphones, ThumbsUp } from "lucide-react"
import Link from "next/link"

import { Button } from "@/components/ui/button"
import { useCurrentUser } from "@/hooks/use-current-user"

export function DashboardHeader() {
  const { data: user, isLoading } = useCurrentUser()

  return (
    <div className="flex items-start justify-between">
      <div className="space-y-1">
        <p className="text-sm text-muted-foreground">Nice to see you</p>
        <h1 className="text-2xl lg:text-3xl font-semibold tracking-tight">
          {isLoading ? "..." : (user?.name ?? "there")}
        </h1>
      </div>

      <div className="lg:flex items-center gap-3 hidden">
        <Button
          variant="outline"
          size="sm"
          asChild>
          <Link href="mailto:business@codewithantonio.com">
            <ThumbsUp />
            <span className="hidden lg:block">Feedback</span>
          </Link>
        </Button>
        <Button
          variant="outline"
          size="sm"
          asChild>
          <Link href="mailto:business@codewithantonio.com">
            <Headphones />
            <span className="hidden lg:block">Need help?</span>
          </Link>
        </Button>
      </div>
    </div>
  )
}
