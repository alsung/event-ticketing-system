'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { Field } from '@/components/ui/Field';
import { Button } from '@/components/ui/Button';
import { Alert } from '@/components/ui/Alert';

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL ?? 'http://localhost:8000';
const MIN_PASSWORD = 8;

export default function RegisterPage() {
    const router = useRouter();
    const [email, setEmail] = useState('');
    const [fullName, setFullName] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [success, setSuccess] = useState('');
    const [submitting, setSubmitting] = useState(false);
    // Validate on blur rather than on every keystroke: an error that appears
    // while someone is still typing their password is noise, not help.
    const [touchedPassword, setTouchedPassword] = useState(false);

    const passwordError =
        touchedPassword && password.length > 0 && password.length < MIN_PASSWORD
            ? `Use at least ${MIN_PASSWORD} characters.`
            : undefined;

    const handleRegister = async (e: React.FormEvent) => {
        e.preventDefault();
        if (submitting) return;

        setError('');
        setSuccess('');

        if (password.length < MIN_PASSWORD) {
            setTouchedPassword(true);
            return;
        }

        setSubmitting(true);
        try {
            const res = await fetch(`${GATEWAY}/users/register`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ email, password, full_name: fullName }),
            });

            if (!res.ok) {
                // 409 is a duplicate email, which the person can fix; anything
                // else is ours to own. Both messages say what to do next.
                setError(res.status === 409
                    ? 'An account with that email already exists. Try signing in instead.'
                    : 'Could not create your account. Please try again.');
                return;
            }

            setSuccess('Account created. Taking you to sign in…');
            setTimeout(() => router.push('/login'), 1200);
        } catch {
            setError('Could not reach the server. Check your connection and try again.');
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div className="mx-auto flex max-w-sm flex-col gap-6 py-8">
            <div className="flex flex-col gap-1.5">
                <h1>Create an account</h1>
                <p className="text-[15px] text-text-secondary">
                    Book tickets and keep them all in one place.
                </p>
            </div>

            <form onSubmit={handleRegister} className="flex flex-col gap-4" noValidate>
                {error && <Alert>{error}</Alert>}
                {success && <Alert tone="success">{success}</Alert>}

                <Field
                    label="Full name"
                    value={fullName}
                    onChange={setFullName}
                    autoComplete="name"
                    disabled={submitting}
                />

                <Field
                    label="Email"
                    type="email"
                    value={email}
                    onChange={setEmail}
                    required
                    autoComplete="email"
                    disabled={submitting}
                />

                <div onBlur={() => setTouchedPassword(true)}>
                    <Field
                        label="Password"
                        type="password"
                        value={password}
                        onChange={setPassword}
                        required
                        autoComplete="new-password"
                        hint={`At least ${MIN_PASSWORD} characters.`}
                        error={passwordError}
                        disabled={submitting}
                    />
                </div>

                <Button type="submit" loading={submitting} fullWidth>
                    {submitting ? 'Creating account…' : 'Create account'}
                </Button>
            </form>

            <p className="text-center text-sm text-text-secondary">
                Already have an account?{' '}
                <Link href="/login" className="font-medium text-accent hover:underline">
                    Sign in
                </Link>
            </p>
        </div>
    );
}
