"use client"

import Link from "next/link"
import * as z from "zod"
import { useForm } from "@tanstack/react-form"

import { LogoIcon } from "@/components/logo"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { authRpcClient } from "@/lib/rpc"
import { setAuthCookies } from "@/actions/auth"
import { useAuth } from "@/lib/auth-context"
import { useRouter } from "next/navigation"

const formSchema = z.object({
  email: z.string().email("Please enter a valid email address"),
  password: z.string().min(6, "Password must be at least 6 characters"),
})

function getErrorMessage(error: unknown): string {
  if (typeof error === "string") return error
  if (error && typeof error === "object" && "message" in error) {
    const msg = (error as { message?: unknown }).message
    if (typeof msg === "string") return msg
  }
  return "Invalid value"
}

export default function LoginForm() {
  const { setAccessToken } = useAuth()
  const router = useRouter()
  const form = useForm({
    defaultValues: {
      email: "",
      password: "",
    },
    validators: {
      onSubmit: ({ value }) => {
        const result = formSchema.safeParse(value)
        if (!result.success) {
          return result.error.flatten().fieldErrors
        }
        return undefined
      },
    },
    onSubmit: async ({ value }) => {
      try {
        const response = await authRpcClient.login({
          email: value.email,
          password: value.password,
        })

        // Persist refresh token in HTTP-only cookie
        await setAuthCookies(response.refreshToken)

        // Persist access token in memory
        setAccessToken(response.accessToken)

        // Redirect to dashboard after successful authentication
        router.push("/dashboard")
      } catch (error) {
        console.error("Login failed:", error)
      }
    },
  })

  return (
    <main className="flex min-h-svh items-center justify-center bg-zinc-50 px-4 py-10 md:py-16 dark:bg-transparent">
      <form
        onSubmit={(e) => {
          e.preventDefault()
          e.stopPropagation()
          void form.handleSubmit()
        }}
        className="bg-muted w-full max-w-sm overflow-hidden rounded-[calc(var(--radius)+.125rem)] border shadow-md shadow-zinc-950/5 dark:[--color-muted:var(--color-zinc-900)]">
        <div className="bg-card -m-px rounded-[calc(var(--radius)+.125rem)] border p-6 sm:p-8">
          <div className="text-center">
            <Link
              href="/"
              aria-label="Go home"
              className="mx-auto inline-flex">
              <LogoIcon />
            </Link>
            <h1 className="mt-4 text-xl font-semibold">Sign in to Tailark</h1>
            <p className="text-muted-foreground mt-1 text-sm">Welcome back! Sign in to continue</p>
          </div>

          <div className="mt-6 space-y-5">
            <form.Field
              name="email"
              children={(field) => (
                <div className="space-y-2">
                  <Label
                    htmlFor={field.name}
                    className="text-sm">
                    Email
                  </Label>
                  <Input
                    id={field.name}
                    name={field.name}
                    type="email"
                    autoComplete="email"
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="you@example.com"
                    aria-invalid={field.state.meta.errors.length > 0}
                  />
                  {field.state.meta.errors.length > 0 ? (
                    <p className="text-destructive text-xs">
                      {field.state.meta.errors.map(getErrorMessage).join(", ")}
                    </p>
                  ) : null}
                </div>
              )}
            />

            <form.Field
              name="password"
              children={(field) => (
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <Label
                      htmlFor={field.name}
                      className="text-sm">
                      Password
                    </Label>
                    <Link
                      href="#"
                      className="text-muted-foreground hover:text-foreground text-xs underline-offset-4 hover:underline">
                      Forgot your password?
                    </Link>
                  </div>
                  <Input
                    id={field.name}
                    name={field.name}
                    type="password"
                    autoComplete="current-password"
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="••••••••"
                    aria-invalid={field.state.meta.errors.length > 0}
                  />
                  {field.state.meta.errors.length > 0 ? (
                    <p className="text-destructive text-xs">
                      {field.state.meta.errors.map(getErrorMessage).join(", ")}
                    </p>
                  ) : null}
                </div>
              )}
            />

            <form.Subscribe
              selector={(state) => [state.canSubmit, state.isSubmitting]}
              children={([canSubmit, isSubmitting]) => (
                <Button
                  className="w-full"
                  type="submit"
                  disabled={!canSubmit}>
                  {isSubmitting ? "Signing in..." : "Sign in"}
                </Button>
              )}
            />
          </div>

          <div className="my-6 grid grid-cols-[1fr_auto_1fr] items-center gap-3">
            <hr className="border-border border-dashed" />
            <span className="text-muted-foreground text-xs">Or continue with</span>
            <hr className="border-border border-dashed" />
          </div>

          <div className="grid grid-cols-2 gap-3">
            <Button
              type="button"
              variant="outline"
              className="gap-2">
              <span>Google</span>
            </Button>
            <Button
              type="button"
              variant="outline"
              className="gap-2">
              <span>Microsoft</span>
            </Button>
          </div>
        </div>

        <div className="p-3">
          <p className="text-muted-foreground text-center text-sm">
            Don&apos;t have an account?
            <Button
              asChild
              variant="link"
              className="px-2">
              <Link href="register">Create account</Link>
            </Button>
          </p>
        </div>
      </form>
    </main>
  )
}
