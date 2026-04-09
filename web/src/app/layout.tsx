import type { Metadata } from "next"
import { Geist, Geist_Mono, Inter } from "next/font/google"
import "./globals.css"
import { cn } from "@/lib/utils"
import { AuthProvider } from "@/lib/auth-context"
import { QueryProvider } from "@/lib/react-query"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import { NuqsAdapter } from "nuqs/adapters/next/app"

const inter = Inter({ subsets: ["latin"], variable: "--font-sans" })

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
})

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
})

export const metadata: Metadata = {
  title: {
    default: "Resonance",
    template: "%s | Resonance",
  },
  description: "AI-powered text-to-speech and voice cloning platform",
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html
      lang="en"
      className={cn(
        "h-full",
        "antialiased",
        geistSans.variable,
        geistMono.variable,
        "font-sans",
        inter.variable,
      )}>
      <body className="min-h-full flex flex-col">
        <QueryProvider>
          <AuthProvider>
            <NuqsAdapter>
              <Toaster richColors />
              <TooltipProvider>{children}</TooltipProvider>
            </NuqsAdapter>
          </AuthProvider>
        </QueryProvider>
      </body>
    </html>
  )
}
