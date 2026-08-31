'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { setToken } from '@/context/UserContext';
import { Field } from '@/components/ui/Field';
import { Button } from '@/components/ui/Button';
import { Alert } from '@/components/ui/Alert';

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL ?? 'http://localhost:8000';

export default function LoginPage() {
    const router = useRouter();
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [submitting, setSubmitting] = useState(false);

    const handleLogin = async (e: React.FormEvent) => {
        e.preventDefault();
        // Guard against the double submit rather than relying on the disabled
        // attribute alone: a fast second Enter can land before React re-renders.
        if (submitting) return;

        setError('');
        setSubmitting(true);

        try {
            const res = await fetch(`${GATEWAY}/users/login`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, password }),
            });

            if (!res.ok) {
                // The API answers 401 with plain text, so read it as text and
                // fall back to something a person can act on.
                setError(res.status === 401
                    ? 'That email and password do not match. Check both and try again.'
                    : 'Could not sign you in. Please try again.');
                return;
            }

            const data = await res.json();
            setToken(data.token);
            router.push('/events');
        } catch {
            setError('Could not reach the server. Check your connection and try again.');
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div className="mx-auto flex max-w-sm flex-col gap-6 py-8">
            <div className="flex flex-col gap-1.5">
                <h1>Welcome back</h1>
                <p className="text-[15px] text-text-secondary">
                    Sign in to see your tickets and book new events.
                </p>
            </div>

            <form onSubmit={handleLogin} className="flex flex-col gap-4" noValidate>
                {error && <Alert>{error}</Alert>}

                <Field
                    label="Email"
                    type="email"
                    value={email}
                    onChange={setEmail}
                    required
                    autoComplete="email"
                    disabled={submitting}
                />

                <Field
                    label="Password"
                    type="password"
                    value={password}
                    onChange={setPassword}
                    required
                    autoComplete="current-password"
                    disabled={submitting}
                />

                <Button type="submit" loading={submitting} fullWidth>
                    {submitting ? 'Signing in…' : 'Sign in'}
                </Button>
            </form>

            <p className="text-center text-sm text-text-secondary">
                New here?{' '}
                <Link href="/register" className="font-medium text-accent hover:underline">
                    Create an account
                </Link>
            </p>
        </div>
    );
}
