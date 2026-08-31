'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useUser } from '@/context/UserContext';
import { Button } from '@/components/ui/Button';

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL ?? 'http://localhost:8000';

interface AvailableTicket {
    id: string;
    price: number;
}

type State =
    | { kind: 'loading' }
    | { kind: 'ready'; count: number; price: number }
    | { kind: 'sold-out' }
    | { kind: 'error' };

/** Availability is a client component because /tickets/available requires the
 *  caller's token, which lives in localStorage. Keeping it separate lets the
 *  rest of the page stay a Server Component. */
export function Availability({ eventId, eventName }: { eventId: string; eventName: string }) {
    const { user } = useUser();
    const [state, setState] = useState<State>({ kind: 'loading' });

    useEffect(() => {
        // Signed-out is derived from `user` at render time rather than stored:
        // setting it here would be a synchronous setState inside an effect,
        // which triggers a cascading render.
        if (!user) return;

        let cancelled = false;

        const load = async () => {
            try {
                const token = localStorage.getItem('token');
                const res = await fetch(
                    `${GATEWAY}/tickets/available?event_id=${eventId}`,
                    { headers: { Authorization: `Bearer ${token}` } },
                );
                if (!res.ok) throw new Error(String(res.status));

                const tickets: AvailableTicket[] = await res.json();
                // Guard against a state update after the component unmounts or
                // the event id changes mid-flight.
                if (cancelled) return;

                if (!Array.isArray(tickets) || tickets.length === 0) {
                    setState({ kind: 'sold-out' });
                    return;
                }
                setState({ kind: 'ready', count: tickets.length, price: tickets[0].price });
            } catch {
                if (!cancelled) setState({ kind: 'error' });
            }
        };

        void load();
        return () => { cancelled = true; };
    }, [eventId, user]);

    // Signed-out visitors can read the event but not its inventory. That is a
    // prompt to sign in, not an error.
    if (!user) {
        return (
            <aside className="flex h-fit flex-col gap-4 rounded-lg border border-border bg-surface p-5 shadow-e2">
                <h2 className="t-label text-text-muted">Tickets</h2>
                <p className="text-[15px] text-text-secondary">
                    Sign in to see availability and book a ticket.
                </p>
                <Link
                    href="/login"
                    className="inline-flex h-11 items-center justify-center rounded-md bg-accent px-4 text-[15px] font-medium text-on-accent transition-[background-color,transform] duration-[120ms] hover:bg-accent-hover active:scale-[.98]"
                >
                    Sign in
                </Link>
            </aside>
        );
    }

    return (
        <aside className="flex h-fit flex-col gap-4 rounded-lg border border-border bg-surface p-5 shadow-e2">
            <h2 className="t-label text-text-muted">Tickets</h2>

            {state.kind === 'loading' && (
                <div className="flex animate-pulse flex-col gap-3">
                    <div className="h-8 w-28 rounded bg-surface-sunken" />
                    <div className="h-4 w-36 rounded bg-surface-sunken" />
                    <div className="h-11 w-full rounded-md bg-surface-sunken" />
                </div>
            )}

            {state.kind === 'ready' && (
                <>
                    <p className="tnum text-3xl font-semibold tracking-[-.02em] text-text">
                        ${state.price.toFixed(2)}
                    </p>
                    <p className="flex items-center gap-2 text-[15px] text-text-secondary">
                        <span className="inline-block h-2 w-2 rounded-full bg-success" aria-hidden="true" />
                        {/* The count is stated in words as well as colour, since
                            colour alone is not an accessible signal. */}
                        <span className="tnum">{state.count}</span>
                        {state.count === 1 ? 'ticket left' : 'tickets available'}
                    </p>
                    <Link
                        href={`/events/${eventId}/checkout`}
                        className="inline-flex h-11 items-center justify-center rounded-md bg-accent px-4 text-[15px] font-medium text-on-accent transition-[background-color,transform] duration-[120ms] hover:bg-accent-hover active:scale-[.98]"
                    >
                        Buy a ticket
                    </Link>
                </>
            )}

            {state.kind === 'sold-out' && (
                <>
                    <p className="flex items-center gap-2 text-[15px] font-medium text-text">
                        <span className="inline-block h-2 w-2 rounded-full bg-danger" aria-hidden="true" />
                        Sold out
                    </p>
                    <p className="text-[15px] text-text-secondary">
                        Every ticket for {eventName} has been claimed. Cancellations return
                        seats to the pool, so it is worth checking back.
                    </p>
                </>
            )}

            {state.kind === 'error' && (
                <>
                    <p className="text-[15px] text-text-secondary">
                        Could not load availability.
                    </p>
                    <Button variant="secondary" onClick={() => setState({ kind: 'loading' })}>
                        Try again
                    </Button>
                </>
            )}
        </aside>
    );
}
