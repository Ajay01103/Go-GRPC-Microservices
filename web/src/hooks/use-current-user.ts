import { useQuery } from "@tanstack/react-query"
import { useAuth } from "@/lib/auth-context"
import { rpcClient } from "@/lib/rpc"

export function useCurrentUser() {
  const { accessToken } = useAuth()

  return useQuery({
    queryKey: ["currentUser", accessToken],
    queryFn: async () => {
      if (!accessToken) return null

      const response = await rpcClient.getCurrentUser(
        {},
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
          },
        },
      )

      return response
    },
    enabled: !!accessToken,
  })
}
