'use client';

import { useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { loadStripe } from '@stripe/stripe-js';
import { Elements, CardElement, useStripe, useElements } from '@stripe/react-stripe-js';
import { useUser } from '@/context/UserContext';
import { Button } from '@/components/ui/Button';
import { Alert } from '@/components/ui/Alert';

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL ?? 'http://localhost:8000';
const PUBLISHABLE_KEY = process.env.NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY ?? '';

// loadStripe is called once at module scope, not per render: it injects a script
// tag, and calling it on every render would reload Stripe repeatedly.
const stripePromise = PUBLISHABLE_KEY ? loadStripe(PUBLISHABLE_KEY) : null;

/** The card field, styled from the same tokens as the rest of the app. Stripe
 *  renders it in a cross-origin iframe, so it cannot inherit CSS — every value
 *  has to be passed explicitly. */
function cardStyle(): Record<string, unknown> {
    const read = (name: string) =>
        getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return {
        style: {
            base: {
                color: read('--text') || '#0F172A',
                fontFamily: 'Inter, system-ui, sans-serif',
                fontSize: '15px',
                '::placeholder': { color: read('--text-muted') || '#64748B' },
            },
            invalid: { color: read('--danger') || '#B91C1C' },
        },
    };
}

function PaymentForm({ eventId, eventName }: { eventId: string; eventName: string }) {
    const stripe = useStripe();
    const elements = useElements();
    const router = useRouter();

    const [error, setError] = useState('');
    const [submitting, setSubmitting] = useState(false);

    // One key per mounted form. A retry after a network failure reuses it, so
    // the server replays the original response instead of claiming a second
    // seat -- which is the whole reason the endpoint accepts the header.
    //
    // Generated on first submit rather than during render: randomUUID is impure,
    // and a value that differs between renders would defeat the purpose anyway.
    const idempotencyKey = useRef<string | null>(null);

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (submitting || !stripe || !elements) return;

        setError('');
        setSubmitting(true);

        if (idempotencyKey.current === null) {
            idempotencyKey.current = globalThis.crypto.randomUUID();
        }

        try {
            const card = elements.getElement(CardElement);
            if (!card) throw new Error('card element missing');

            // The card details never touch our server: Stripe exchanges them for
            // a PaymentMethod id in the browser, and only that id is sent on.
            const { error: pmError, paymentMethod } = await stripe.createPaymentMethod({
                type: 'card',
                card,
            });

            if (pmError) {
                setError(pmError.message ?? 'Could not read those card details.');
                return;
            }

            const res = await fetch(`${GATEWAY}/tickets/purchase`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    Authorization: `Bearer ${localStorage.getItem('token')}`,
                    'Idempotency-Key': idempotencyKey.current,
                },
                body: JSON.stringify({
                    event_id: eventId,
                    payment_method_id: paymentMethod.id,
                }),
            });

            if (res.ok) {
                const data = await res.json();
                router.push(`/tickets?purchased=${data.ticket_id}`);
                return;
            }

            // Each status is a different situation and deserves a different
            // sentence. Collapsing them into "something went wrong" throws away
            // the distinctions the API works to preserve.
            switch (res.status) {
                case 409:
                    setError(`${eventName} sold out while you were checking out. Nothing was charged.`);
                    break;
                case 402:
                    setError('Your card was declined. Try a different card.');
                    break;
                case 422:
                    setError('This checkout was already submitted with different details. Reload the page and try again.');
                    break;
                case 502:
                    setError('The payment provider is unreachable. Nothing was charged — please try again shortly.');
                    break;
                case 401:
                    setError('Your session expired. Sign in again to complete this purchase.');
                    break;
                default:
                    setError('Could not complete the purchase. Nothing was charged.');
            }
        } catch {
            // A network failure leaves the outcome genuinely unknown: the request
            // may have been processed. Say so rather than implying it failed.
            setError('Lost connection before we heard back. Check My tickets before trying again.');
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
            {error && <Alert>{error}</Alert>}

            <div className="flex flex-col gap-2">
                <label htmlFor="card" className="text-sm font-medium text-text">
                    Card details
                </label>
                <div
                    id="card"
                    className="rounded-md border border-border-strong bg-surface px-3 py-3.5 transition-[border-color] duration-[120ms] focus-within:border-accent"
                >
                    <CardElement options={cardStyle()} />
                </div>
                <p className="text-[13px] text-text-muted">
                    Test mode. Use <code className="rounded bg-surface-sunken px-1.5 py-0.5">4242 4242 4242 4242</code>,
                    any future expiry, any CVC.
                </p>
            </div>

            <Button type="submit" loading={submitting} disabled={!stripe} fullWidth>
                {submitting ? 'Processing…' : 'Pay and get my ticket'}
            </Button>

            <p className="text-center text-[13px] text-text-muted">
                Your card is charged once. Retrying a failed request will not charge you twice.
            </p>
        </form>
    );
}

export function CheckoutForm({ eventId, eventName }: { eventId: string; eventName: string }) {
    const { user } = useUser();

    if (!user) {
        return (
            <div className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-6">
                <p className="text-[15px] text-text-secondary">
                    You need to be signed in to buy a ticket.
                </p>
                <Link
                    href="/login"
                    className="inline-flex h-11 w-fit items-center rounded-md bg-accent px-5 text-[15px] font-medium text-on-accent transition-[background-color,transform] duration-[120ms] hover:bg-accent-hover active:scale-[.98]"
                >
                    Sign in
                </Link>
            </div>
        );
    }

    // Without a publishable key the page would render a dead card field. Saying
    // what is missing is more useful than a form that silently cannot submit.
    if (!stripePromise) {
        return (
            <Alert>
                Stripe is not configured. Set NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY to enable checkout.
            </Alert>
        );
    }

    return (
        <div className="rounded-lg border border-border bg-surface p-6 shadow-e2">
            <Elements stripe={stripePromise}>
                <PaymentForm eventId={eventId} eventName={eventName} />
            </Elements>
        </div>
    );
}
