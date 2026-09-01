'use client';

import { use, useCallback, useEffect, useState } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useUser } from '@/context/UserContext';
import { Field } from '@/components/ui/Field';
import { Button } from '@/components/ui/Button';
import { Alert } from '@/components/ui/Alert';
import { EventForm, toLocalInput } from '../EventForm';

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL ?? 'http://localhost:8000';

interface OrganizerEvent {
    id: string;
    name: string;
    description: string | null;
    location: string | null;
    start_time: string;
    end_time: string;
    total_tickets: number;
    sold_tickets: number;
    available_tickets: number;
    revenue: number;
}

export default function ManageEventPage({ params }: { params: Promise<{ id: string }> }) {
    // `use` unwraps the params promise in a client component.
    const { id } = use(params);
    const { user } = useUser();
    const router = useRouter();

    const [event, setEvent] = useState<OrganizerEvent | null>(null);
    const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading');
    const [saved, setSaved] = useState(false);

    const [price, setPrice] = useState('49.99');
    const [quantity, setQuantity] = useState('50');
    const [minting, setMinting] = useState(false);
    const [mintError, setMintError] = useState('');
    const [minted, setMinted] = useState(0);

    const load = useCallback(async () => {
        try {
            // The organiser listing already carries the sales figures, so the
            // page reads its own row from there rather than adding an endpoint.
            const res = await fetch(`${GATEWAY}/organizer/events`, {
                headers: { Authorization: `Bearer ${localStorage.getItem('token')}` },
            });
            if (!res.ok) throw new Error(String(res.status));
            const all: OrganizerEvent[] = await res.json();
            const found = all.find((e) => e.id === id) ?? null;
            setEvent(found);
            setStatus(found ? 'ready' : 'error');
        } catch {
            setStatus('error');
        }
    }, [id]);

    useEffect(() => {
        // eslint-disable-next-line react-hooks/set-state-in-effect
        if (user) void load();
    }, [user, load]);

    const mint = async (e: React.FormEvent) => {
        e.preventDefault();
        if (minting) return;
        setMintError('');

        const qty = Number(quantity);
        const cost = Number(price);
        if (!Number.isInteger(qty) || qty < 1 || qty > 1000) {
            setMintError('Enter a whole number of tickets between 1 and 1000.'); return;
        }
        if (!(cost >= 0)) { setMintError('Enter a price of zero or more.'); return; }

        setMinting(true);
        try {
            const res = await fetch(`${GATEWAY}/tickets/create`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    Authorization: `Bearer ${localStorage.getItem('token')}`,
                },
                body: JSON.stringify({ event_id: id, price: cost, quantity: qty }),
            });

            if (!res.ok) {
                setMintError(res.status === 403
                    ? 'This event is not yours to add tickets to.'
                    : 'Could not add the tickets. Please try again.');
                return;
            }

            setMinted(qty);
            await load();
        } catch {
            setMintError('Could not reach the server. Check your connection and try again.');
        } finally {
            setMinting(false);
        }
    };

    if (!user) {
        return (
            <div className="rounded-lg border border-border bg-surface p-6">
                <p className="text-[15px] text-text-secondary">Sign in to manage this event.</p>
            </div>
        );
    }

    if (status === 'loading') {
        return <div className="h-64 animate-pulse rounded-lg bg-surface-sunken" />;
    }

    if (status === 'error' || !event) {
        return (
            <div className="flex flex-col items-start gap-4">
                <Alert>That event was not found, or is not yours to manage.</Alert>
                <Link href="/organizer" className="text-sm font-medium text-accent hover:underline">
                    Back to your events
                </Link>
            </div>
        );
    }

    const pct = event.total_tickets > 0
        ? Math.round((event.sold_tickets / event.total_tickets) * 100) : 0;

    return (
        <div className="flex flex-col gap-7">
            <nav aria-label="Breadcrumb">
                <Link href="/organizer" className="inline-flex items-center gap-1.5 text-sm text-text-secondary transition-colors duration-[120ms] hover:text-accent">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                        <path d="M15 18l-6-6 6-6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                    Your events
                </Link>
            </nav>

            <div className="flex flex-wrap items-end justify-between gap-3">
                <h1>{event.name}</h1>
                <Link href={`/events/${event.id}`} className="text-sm font-medium text-accent hover:underline">
                    View public page
                </Link>
            </div>

            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                {[
                    { v: String(event.total_tickets), k: 'tickets' },
                    { v: String(event.sold_tickets), k: 'sold' },
                    { v: String(event.available_tickets), k: 'available' },
                    { v: `$${event.revenue.toFixed(2)}`, k: 'revenue', good: true },
                ].map((s) => (
                    <div key={s.k} className="rounded-lg border border-border bg-surface p-4">
                        <div className={`tnum text-xl font-semibold ${s.good ? 'text-success' : 'text-text'}`}>{s.v}</div>
                        <div className="text-[13px] text-text-muted">{s.k}</div>
                    </div>
                ))}
            </div>

            {event.total_tickets > 0 && (
                <div>
                    <div className="h-1.5 overflow-hidden rounded-full bg-surface-sunken">
                        <div className="h-full rounded-full bg-accent transition-[width] duration-[220ms]"
                             style={{ width: `${pct}%` }} />
                    </div>
                    <p className="tnum mt-1.5 text-[13px] text-text-muted">{pct}% sold</p>
                </div>
            )}

            <section className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-6">
                <h2>Add tickets</h2>
                <p className="text-[15px] text-text-secondary">
                    Tickets are minted as individual seats at a fixed price. An event with none
                    cannot be bought.
                </p>

                {minted > 0 && (
                    <Alert tone="success">
                        Added {minted} {minted === 1 ? 'ticket' : 'tickets'}.
                    </Alert>
                )}
                {mintError && <Alert>{mintError}</Alert>}

                <form onSubmit={mint} className="flex flex-col gap-4" noValidate>
                    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                        <Field label="Price each" type="number" value={price} onChange={setPrice}
                               required disabled={minting} hint="In dollars." />
                        <Field label="How many" type="number" value={quantity} onChange={setQuantity}
                               required disabled={minting} hint="Up to 1000 at a time." />
                    </div>
                    <Button type="submit" loading={minting} variant="secondary">
                        {minting ? 'Adding…' : 'Add tickets'}
                    </Button>
                </form>
            </section>

            <section className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-6">
                <h2>Event details</h2>
                {saved && <Alert tone="success">Saved.</Alert>}
                <EventForm
                    eventId={event.id}
                    submitLabel="Save changes"
                    initial={{
                        name: event.name,
                        description: event.description ?? '',
                        location: event.location ?? '',
                        start_time: toLocalInput(event.start_time),
                        end_time: toLocalInput(event.end_time),
                    }}
                    onDone={() => { setSaved(true); void load(); router.refresh(); }}
                />
            </section>
        </div>
    );
}
