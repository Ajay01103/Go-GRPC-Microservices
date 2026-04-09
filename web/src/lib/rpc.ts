import { createClient } from "@connectrpc/connect"
import { createConnectTransport } from "@connectrpc/connect-web"

import { AuthService } from "../gen/pb/auth_connect"
import { GenerationService } from "../gen/pb/generation_connect"
import { VoiceService } from "../gen/pb/voice_connect"

const authBaseUrl = process.env.NEXT_PUBLIC_AUTH_RPC_URL ?? "http://localhost:50051"
const voiceBaseUrl = process.env.NEXT_PUBLIC_VOICE_RPC_URL ?? "http://localhost:50052"
const generationBaseUrl = process.env.NEXT_PUBLIC_GENERATION_RPC_URL ?? "http://localhost:50053"

const authTransport = createConnectTransport({
  baseUrl: authBaseUrl,
})

const voiceTransport = createConnectTransport({
  baseUrl: voiceBaseUrl,
})

const generationTransport = createConnectTransport({
  baseUrl: generationBaseUrl,
})

export const authRpcClient = createClient(AuthService, authTransport)
export const voiceRpcClient = createClient(VoiceService, voiceTransport)
export const generationRpcClient = createClient(GenerationService, generationTransport)

// Backward-compatible alias for existing auth imports.
export const rpcClient = authRpcClient
