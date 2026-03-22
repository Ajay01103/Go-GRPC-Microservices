import { createPromiseClient } from "@connectrpc/connect"
import { createConnectTransport } from "@connectrpc/connect-web"

import { AuthService } from "../gen/pb/auth_connect"

const transport = createConnectTransport({
  baseUrl: "http://localhost:50051", // Typical gRPC port used in this boilerplate
})

export const rpcClient = createPromiseClient(AuthService, transport)
