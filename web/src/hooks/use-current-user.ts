import { useQuery } from "@tanstack/react-query"
import { useAuth } from "@/lib/auth-context"
import { authRpcClient } from "@/lib/rpc"

export type CurrentUser = {
  userId: string
  email: string
  name: string
}

export function useCurrentUser() {
  const { accessToken } = useAuth()

  return useQuery({
    queryKey: ["currentUser", !!accessToken],
    enabled: !!accessToken,
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: 1,
    queryFn: async (): Promise<CurrentUser | null> => {
      if (!accessToken) return null

      const response = await authRpcClient.getCurrentUser(
        {},
        {
          headers: {
            Authorization: `Bearer ${accessToken}`,
          },
        },
      )

      return response as CurrentUser
    },
  })
}
