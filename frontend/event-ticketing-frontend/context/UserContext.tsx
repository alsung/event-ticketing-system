'use client';

import { createContext, useCallback, useContext, useMemo, useSyncExternalStore, ReactNode } from "react";
import { useRouter } from "next/navigation";
import { jwtDecode } from "jwt-decode";

interface User {
    email: string;
    user_id: string;
}

interface JwtPayload {
    email: string;
    user_id: string;
    exp: number;
}

interface UserContextType {
    user: User | null;
    setUser: (user: User | null) => void;
    signOut: () => void;
}

const UserContext = createContext<UserContextType | undefined>(undefined);

const TOKEN_KEY = "token";

// localStorage is state living outside React, mutated by other tabs and by our
// own login flow. useSyncExternalStore is the hook built for that case: it needs
// a subscribe function, a client snapshot and a server snapshot. Reading it in an
// effect and calling setState instead causes the cascading render that
// react-hooks/set-state-in-effect flags, and lazy useState init cannot work here
// because localStorage does not exist during server rendering.

const listeners = new Set<() => void>();

function emit() {
    listeners.forEach((l) => l());
}

function subscribe(onStoreChange: () => void) {
    listeners.add(onStoreChange);
    // Keep other tabs in sync: the storage event fires only in *other* documents.
    window.addEventListener("storage", onStoreChange);
    return () => {
        listeners.delete(onStoreChange);
        window.removeEventListener("storage", onStoreChange);
    };
}

function getSnapshot(): string | null {
    return localStorage.getItem(TOKEN_KEY);
}

// The server has no token, so it always renders the signed-out tree; React swaps
// in the client snapshot after hydration.
function getServerSnapshot(): string | null {
    return null;
}

export function setToken(token: string | null) {
    if (token === null) {
        localStorage.removeItem(TOKEN_KEY);
    } else {
        localStorage.setItem(TOKEN_KEY, token);
    }
    emit();
}

export const UserProvider = ({ children }: { children: ReactNode }) => {
    const router = useRouter();
    const token = useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);

    const user = useMemo<User | null>(() => {
        if (!token) return null;
        try {
            const decoded = jwtDecode<JwtPayload>(token);
            // Expiry is deliberately not checked here. useMemo runs during
            // render and must be pure, so calling Date.now() would make the
            // same inputs produce different output. The server rejects expired
            // tokens regardless, which is the authority that matters.
            return { email: decoded.email, user_id: decoded.user_id };
        } catch {
            return null;
        }
    }, [token]);

    const setUser = useCallback((_next: User | null) => {
        // Identity is derived from the stored token, so callers change it by
        // storing or clearing a token rather than by setting user state.
        if (_next === null) setToken(null);
    }, []);

    const signOut = useCallback(() => {
        setToken(null);
        router.push("/login");
    }, [router]);

    const value = useMemo(() => ({ user, setUser, signOut }), [user, setUser, signOut]);

    return <UserContext.Provider value={value}>{children}</UserContext.Provider>;
};

export const useUser = () => {
    const context = useContext(UserContext);
    if (!context) throw new Error("useUser must be used within UserProvider");
    return context;
};
