import { createClient, type Interceptor } from "@connectrpc/connect"
import { createConnectTransport } from "@connectrpc/connect-web"
import { tokenStore } from "@/lib/token-store"

import { AuthService } from "../gen/pb/auth_connect"
import { GenerationService } from "../gen/pb/generation_connect"
import { VoiceService } from "../gen/pb/voice_connect"

const authBaseUrl = process.env.NEXT_PUBLIC_AUTH_RPC_URL ?? "http://localhost:50051"
const voiceBaseUrl = process.env.NEXT_PUBLIC_VOICE_RPC_URL ?? "http://localhost:50052"
const generationBaseUrl =
  process.env.NEXT_PUBLIC_GENERATION_RPC_URL ?? "http://localhost:50053"

const bearerAuthInterceptor: Interceptor = (next) => async (req) => {
  const token = typeof tokenStore.get === "function" ? tokenStore.get() : null
  if (token) {
    req.header.set("Authorization", `Bearer ${token}`)
  }

  return next(req)
}

const authTransport = createConnectTransport({
  baseUrl: authBaseUrl,
})

const browserAuthTransport = createConnectTransport({
  baseUrl: authBaseUrl,
  interceptors: [bearerAuthInterceptor],
})

const voiceTransport = createConnectTransport({
  baseUrl: voiceBaseUrl,
  interceptors: [bearerAuthInterceptor],
})

const generationTransport = createConnectTransport({
  baseUrl: generationBaseUrl,
  interceptors: [bearerAuthInterceptor],
})

export const authRpcClient = createClient(AuthService, authTransport)
export const authBrowserRpcClient = createClient(AuthService, browserAuthTransport)
export const voiceRpcClient = createClient(VoiceService, voiceTransport)
export const generationRpcClient = createClient(GenerationService, generationTransport)
