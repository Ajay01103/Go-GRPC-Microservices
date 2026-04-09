"use client"

import Link from "next/link"
import * as z from "zod"
import { useForm } from "@tanstack/react-form"

import { LogoIcon } from "@/components/logo"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { rpcClient } from "@/lib/rpc"
import { setAuthCookies } from "@/actions/auth"
import { useAuth } from "@/lib/auth-context"
import { useRouter } from "next/navigation"

const formSchema = z.object({
  name: z.string().min(2, "Name must be at least 2 characters"),
  email: z.email("Please enter a valid email address"),
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

export default function RegisterPage() {
  const { setAccessToken } = useAuth()
  const router = useRouter()
  const form = useForm({
    defaultValues: {
      name: "",
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
        const response = await rpcClient.register({
          name: value.name,
          email: value.email,
          password: value.password,
        })
        console.log("Register successful:", response)

        // Persist refresh token in HTTP-only cookie
        await setAuthCookies(response.refreshToken)

        // Persist access token in memory
        setAccessToken(response.accessToken)

        // Redirect to dashboard after successful authentication
        router.push("/dashboard")
      } catch (error) {
        console.error("Register failed:", error)
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
            <h1 className="mt-4 text-xl font-semibold">Create your account</h1>
            <p className="text-muted-foreground mt-1 text-sm">Join Tailark and get started</p>
          </div>

          <div className="mt-6 space-y-5">
            <form.Field
              name="name"
              children={(field) => (
                <div className="space-y-2">
                  <Label
                    htmlFor={field.name}
                    className="text-sm">
                    Name
                  </Label>
                  <Input
                    id={field.name}
                    name={field.name}
                    type="text"
                    autoComplete="name"
                    value={field.state.value}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="John Doe"
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
                  <Label
                    htmlFor={field.name}
                    className="text-sm">
                    Password
                  </Label>
                  <Input
                    id={field.name}
                    name={field.name}
                    type="password"
                    autoComplete="new-password"
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
                  {isSubmitting ? "Creating account..." : "Create account"}
                </Button>
              )}
            />
          </div>
        </div>

        <div className="p-3">
          <p className="text-muted-foreground text-center text-sm">
            Already have an account?
            <Button
              asChild
              variant="link"
              className="px-2">
              <Link href="/login">Sign in</Link>
            </Button>
          </p>
        </div>
      </form>
    </main>
  )
}
