'use client';

import { Suspense, useCallback, useEffect, useState } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import { useUser } from '@/context/UserContext';
import { Alert } from '@/components/ui/Alert';
import { Button } from '@/components/ui/Button';
import { TicketCard, type Ticket, type EventSummary } from './TicketCard';

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL ?? 'http://localhost:8000';

type Status = 'loading' | 'ready' | 'error';

function TicketsList() {
    const { user } = useUser();
    const params = useSearchParams();
    // Set by the checkout redirect, so the ticket just bought can be marked.
    const justPurchased = params.get('purchased');

    const [tickets, setTickets] = useState<Ticket[]>([]);
    const [events, setEvents] = useState<Record<string, EventSummary>>({});
    const [status, setStatus] = useState<Status>('loading');

    const load = useCallback(async () => {
        try {
            const token = localStorage.getItem('token');

            // Both at once: /tickets/mine returns event_id but not the event
            // name, and fetching the catalog once beats one request per ticket.
            const [ticketRes, eventRes] = await Promise.all([
                fetch(`${GATEWAY}/tickets/mine`, { headers: { Authorization: `Bearer ${token}` } }),
                fetch(`${GATEWAY}/events`),
            ]);
            if (!ticketRes.ok) throw new Error(String(ticketRes.status));

            const mine = await ticketRes.json();
            setTickets(Array.isArray(mine) ? mine : []);

            if (eventRes.ok) {
                const list = await eventRes.json();
                const map: Record<string, EventSummary> = {};
                for (const e of list ?? []) {
                    map[e.id] = { name: e.name, location: e.location, start_time: e.start_time };
                }
                setEvents(map);
            }
            setStatus('ready');
        } catch {
            setStatus('error');
        }
    }, []);

    // The remedy this rule points at is fetching on the server, which is not
    // available here: /tickets/mine needs the caller's token, and that token
    // lives in localStorage where the server cannot read it. Moving it to an
    // httpOnly cookie would let this become a Server Component, and is already
    // recorded as a known simplification.
    useEffect(() => {
        // eslint-disable-next-line react-hooks/set-state-in-effect
        if (user) void load();
    }, [user, load]);

    const removeTicket = useCallback((id: string) => {
        // Drop it locally rather than refetching: the server has already
        // confirmed, and a refetch would make the card vanish a beat later.
        setTickets((current) => current.filter((t) => t.id !== id));
    }, []);

    if (!user) {
        return (
            <div className="flex flex-col items-start gap-4 rounded-lg border border-border bg-surface p-6">
                <p className="text-[15px] text-text-secondary">Sign in to see your tickets.</p>
                <Link
                    href="/login"
                    className="inline-flex h-11 items-center rounded-md bg-accent px-5 text-[15px] font-medium text-on-accent transition-[background-color,transform] duration-[120ms] hover:bg-accent-hover active:scale-[.98]"
                >
                    Sign in
                </Link>
            </div>
        );
    }

    return (
        <div className="flex flex-col gap-6">
            <div className="flex flex-col gap-1.5">
                <h1>My tickets</h1>
                <p className="text-[15px] text-text-secondary">
                    Show the QR code at the door.
                </p>
            </div>

            {justPurchased && status === 'ready' && (
                <Alert tone="success">
                    Payment received. Your ticket is confirmed and a receipt is on its way.
                </Alert>
            )}

            {status === 'loading' && (
                <ul className="flex flex-col gap-4">
                    {[0, 1].map((i) => (
                        <li key={i} className="flex gap-4 rounded-lg border border-border bg-surface p-5">
                            <div className="h-28 w-28 flex-none animate-pulse rounded-md bg-surface-sunken" />
                            <div className="flex flex-1 animate-pulse flex-col gap-3 py-1">
                                <div className="h-3 w-20 rounded bg-surface-sunken" />
                                <div className="h-5 w-1/2 rounded bg-surface-sunken" />
                                <div className="h-3 w-1/3 rounded bg-surface-sunken" />
                            </div>
                        </li>
                    ))}
                </ul>
            )}

            {status === 'error' && (
                <div className="flex flex-col items-start gap-3">
                    <Alert>Could not load your tickets.</Alert>
                    <Button variant="secondary" onClick={() => { setStatus('loading'); void load(); }}>
                        Try again
                    </Button>
                </div>
            )}

            {status === 'ready' && tickets.length === 0 && (
                <div className="rounded-lg border border-dashed border-border-strong bg-surface-sunken px-6 py-12 text-center">
                    <h2 className="mb-1.5">No tickets yet</h2>
                    <p className="mx-auto mb-5 max-w-sm text-[15px] text-text-secondary">
                        Once you book an event it will appear here with a QR code for admission.
                    </p>
                    <Link
                        href="/events"
                        className="inline-flex h-11 items-center rounded-md bg-accent px-5 text-[15px] font-medium text-on-accent transition-[background-color,transform] duration-[120ms] hover:bg-accent-hover active:scale-[.98]"
                    >
                        Browse events
                    </Link>
                </div>
            )}

            {status === 'ready' && tickets.length > 0 && (
                <ul className="flex flex-col gap-4">
                    {tickets.map((ticket) => (
                        <TicketCard
                            key={ticket.id}
                            ticket={ticket}
                            event={events[ticket.event_id]}
                            highlighted={ticket.id === justPurchased}
                            onCancelled={removeTicket}
                        />
                    ))}
                </ul>
            )}
        </div>
    );
}

export default function TicketsPage() {
    // useSearchParams needs a Suspense boundary, since it opts the tree into
    // client-side rendering that Next streams around.
    return (
        <Suspense fallback={<div className="h-8 w-40 animate-pulse rounded bg-surface-sunken" />}>
            <TicketsList />
        </Suspense>
    );
}
