import { useAuthStore } from "@/stores/auth";
import router from "@/router";
import type { JwtPayload } from "jwt-decode";
import { jwtDecode } from "jwt-decode";
import { authMethod, baseURL, noAuth, logoutPage } from "./constants";
import { StatusError } from "@/api/utils";
import { setSafeTimeout } from "@/api/utils";

export function parseToken(token: string) {
  // falsy or malformed jwt will throw InvalidTokenError
  const data = jwtDecode<JwtPayload & { user: IUser }>(token);

  document.cookie = `auth=${token}; Path=/; SameSite=Strict;`;

  localStorage.setItem("jwt", token);

  const authStore = useAuthStore();
  authStore.jwt = token;
  authStore.setUser(data.user);

  // proxy auth with custom logout subject to unknown external timeout
  if (logoutPage !== "/login" && authMethod === "proxy") {
    console.warn("idle timeout disabled with proxy auth and custom logout");
    return;
  }

  if (authStore.logoutTimer) {
    clearTimeout(authStore.logoutTimer);
  }

  const expiresAt = new Date(data.exp! * 1000);
  const timeout = expiresAt.getTime() - Date.now();
  authStore.setLogoutTimer(
    setSafeTimeout(() => {
      void ensureFreshToken().catch(() => logout("session-expired"));
    }, timeout)
  );
}

export async function validateLogin() {
  try {
    if (localStorage.getItem("jwt")) {
      await renew(<string>localStorage.getItem("jwt"));
    }
  } catch (error) {
    console.warn("Invalid JWT token in storage");
    throw error;
  }
}

export async function login(
  username: string,
  password: string,
  recaptcha: string
) {
  const data = { username, password, recaptcha };

  const res = await fetch(`${baseURL}/api/login`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(data),
  });

  const body = await res.text();

  if (res.status === 200) {
    parseToken(body);
  } else {
    throw new StatusError(
      body || `${res.status} ${res.statusText}`,
      res.status
    );
  }
}

let renewal: Promise<void> | null = null;
let renewalToken = "";
export async function renew(jwt: string) {
  if (renewal && renewalToken === jwt) return renewal;
  renewalToken = jwt;
  const task = (async () => {
    const res = await fetch(`${baseURL}/api/renew`, {
      method: "POST",
      headers: { "X-Auth": jwt },
    });
    const body = await res.text();
    const auth = useAuthStore();
    if (auth.jwt !== jwt && localStorage.getItem("jwt") !== jwt) return;
    if (res.status !== 200)
      throw new StatusError(
        body || `${res.status} ${res.statusText}`,
        res.status
      );
    parseToken(body);
  })();
  renewal = task;
  try {
    await task;
  } finally {
    if (renewal === task) renewal = null;
  }
}
export async function ensureFreshToken() {
  const auth = useAuthStore();
  if (!auth.jwt) return;
  let expires = 0;
  try {
    expires = (jwtDecode<JwtPayload>(auth.jwt).exp || 0) * 1000;
  } catch {
    return;
  }
  if (expires - Date.now() < 5 * 60 * 1000) await renew(auth.jwt);
}
let keepalive: number | undefined;
export function keepSessionAlive() {
  if (keepalive) return;
  keepalive = window.setInterval(() => {
    if (useAuthStore().isLoggedIn && document.visibilityState === "visible")
      void ensureFreshToken().catch(() => logout("session-expired"));
  }, 60_000);
}

export async function signup(username: string, password: string) {
  const data = { username, password };

  const res = await fetch(`${baseURL}/api/signup`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(data),
  });

  if (res.status !== 200) {
    const body = await res.text();
    throw new StatusError(
      body || `${res.status} ${res.statusText}`,
      res.status
    );
  }
}

export function logout(reason?: string) {
  const oldToken = useAuthStore().jwt;
  if (oldToken)
    void fetch(`${baseURL}/api/sessions?id=current`, {
      method: "DELETE",
      headers: { "X-Auth": oldToken },
      keepalive: true,
    }).catch(() => {});
  document.cookie = "auth=; Max-Age=0; Path=/; SameSite=Strict;";

  const authStore = useAuthStore();
  authStore.clearUser();

  localStorage.setItem("jwt", "");
  if (noAuth) {
    window.location.reload();
  } else if (logoutPage !== "/login") {
    document.location.href = `${logoutPage}`;
  } else {
    if (typeof reason === "string" && reason.trim() !== "") {
      router.push({
        path: "/login",
        query: { "logout-reason": reason },
      });
    } else {
      router.push({
        path: "/login",
      });
    }
  }
}
