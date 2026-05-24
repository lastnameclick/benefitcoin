import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { api, setAccessToken, setRefreshToken, getRefreshToken, type Role, type SignupInput, type TokenResponse } from "./api";

interface Session {
  role: Role;
  username: string;
  customerId: string;
  household: string;
}

interface AuthState {
  session: Session | null;
  ready: boolean;
  login: (username: string, password: string) => Promise<void>;
  signup: (input: SignupInput) => Promise<void>;
  logout: () => Promise<void>;
}

const AuthContext = createContext<AuthState>(null as unknown as AuthState);

function toSession(data: TokenResponse): Session {
  return { role: data.role, username: data.username, customerId: data.customer_id, household: data.household };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [ready, setReady] = useState(false);

  // On load, try to resume a session from the stored refresh token.
  useEffect(() => {
    (async () => {
      if (getRefreshToken()) {
        const rt = getRefreshToken()!;
        const res = await fetch("/api/v1/auth/refresh", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ refresh_token: rt }),
        });
        if (res.ok) {
          const data: TokenResponse = await res.json();
          setAccessToken(data.access_token);
          setRefreshToken(data.refresh_token);
          setSession(toSession(data));
        } else {
          setRefreshToken(null);
        }
      }
      setReady(true);
    })();
  }, []);

  const adopt = (data: TokenResponse) => {
    setAccessToken(data.access_token);
    setRefreshToken(data.refresh_token);
    setSession(toSession(data));
  };

  const login = async (username: string, password: string) => adopt(await api.login(username, password));
  const signup = async (input: SignupInput) => adopt(await api.signup(input));

  const logout = async () => {
    await api.logout();
    setSession(null);
  };

  return <AuthContext.Provider value={{ session, ready, login, signup, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  return useContext(AuthContext);
}
