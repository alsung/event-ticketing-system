'use client';

import { useCallback, useEffect, useState } from 'react';
import Link from 'next/link';
import { useUser } from '@/context/UserContext';
import { Alert } from '@/components/ui/Alert';
import { Button } from '@/components/ui/Button';

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL ?? 'http://localhost:8000';

interface OrganizerEvent {
    id: string;
    name: string;
    location: string | null;
    start_time: string;
    total_tickets: number;
    sold_tickets: number;
    available_tickets: number;
    revenue: number;
}

type Status = 'loading' | 'ready' | 'forbidden' | 'error';

export default function OrganizerDashboard() {
    const { user } = useUser();
    const [events, setEvents] = useState<OrganizerEvent[]>([]);
    const [status, setStatus] = useState<Status>('loading');

    const load = useCallback(async () => {
        try {
            const res = await fetch(`${GATEWAY}/organizer/events`, {
                headers: { Authorization: `Bearer ${localStorage.getItem('token')}` },
            });
            // 403 is not an error: it means a perfectly valid account that
            // simply is not an organiser, which deserves an explanation rather
            // than a failure message.
            if (res.status === 403) { setStatus('forbidden'); return; }
            if (!res.ok) throw new Error(String(res.status));
            setEvents(await res.json());
            setStatus('ready');
        } catch {
            setStatus('error');
        }
    }, []);

    useEffect(() => {
        // eslint-disable-next-line react-hooks/set-state-in-effect
        if (user) void load();
    }, [user, load]);

    if (!user) {
        return (
            <div className="flex flex-col items-start gap-4 rounded-lg border border-border bg-surface p-6">
                <p className="text-[15px] text-text-secondary">Sign in to manage your events.</p>
                <Link href="/login" className="inline-flex h-11 items-center rounded-md bg-accent px-5 text-[15px] font-medium text-on-accent transition-[background-color,transform] duration-[120ms] hover:bg-accent-hover active:scale-[.98]">
                    Sign in
                </Link>
            </div>
        );
    }

    const totals = events.reduce(
        (acc, e) => ({
            sold: acc.sold + e.sold_tickets,
            revenue: acc.revenue + e.revenue,
            available: acc.available + e.available_tickets,
        }),
        { sold: 0, revenue: 0, available: 0 },
    );

    return (
        <div className="flex flex-col gap-6">
            <div className="flex flex-wrap items-end justify-between gap-4">
                <div className="flex flex-col gap-1.5">
                    <h1>Your events</h1>
                    <p className="text-[15px] text-text-secondary">
                        Create events, add tickets, and see what has sold.
                    </p>
                </div>
                {status === 'ready' && (
                    <Link href="/organizer/new" className="inline-flex h-11 items-center rounded-md bg-accent px-5 text-[15px] font-medium text-on-accent transition-[background-color,transform] duration-[120ms] hover:bg-accent-hover active:scale-[.98]">
                        New event
                    </Link>
                )}
            </div>

            {status === 'forbidden' && (
                <div className="rounded-lg border border-border bg-surface px-6 py-10 text-center">
                    <h2 className="mb-1.5">This is an organiser area</h2>
                    <p className="mx-auto max-w-md text-[15px] text-text-secondary">
                        Your account can buy tickets but not publish events. Organiser accounts are
                        granted by an admin — in development, sign in as{' '}
                        <code className="rounded bg-surface-sunken px-1.5 py-0.5 text-[13px]">organizer@example.com</code>.
                    </p>
                </div>
            )}

            {status === 'error' && (
                <div className="flex flex-col items-start gap-3">
                    <Alert>Could not load your events.</Alert>
                    <Button variant="secondary" onClick={() => { setStatus('loading'); void load(); }}>
                        Try again
                    </Button>
                </div>
            )}

            {status === 'loading' && (
                <div className="flex flex-col gap-3">
                    {[0, 1, 2].map((i) => (
                        <div key={i} className="h-20 animate-pulse rounded-lg bg-surface-sunken" />
                    ))}
                </div>
            )}

            {status === 'ready' && events.length > 0 && (
                <>
                    {/* Totals first: the summary is what an organiser opens this
                        page for, and the per-event detail is the follow-up. */}
                    <div className="grid grid-cols-3 gap-3">
                        <div className="rounded-lg border border-border bg-surface p-4">
                            <div className="tnum text-2xl font-semibold text-text">{totals.sold}</div>
                            <div className="text-[13px] text-text-muted">tickets sold</div>
                        </div>
                        <div className="rounded-lg border border-border bg-surface p-4">
                            <div className="tnum text-2xl font-semibold text-success">
                                ${totals.revenue.toFixed(2)}
                            </div>
                            <div className="text-[13px] text-text-muted">revenue</div>
                        </div>
                        <div className="rounded-lg border border-border bg-surface p-4">
                            <div className="tnum text-2xl font-semibold text-text">{totals.available}</div>
                            <div className="text-[13px] text-text-muted">still available</div>
                        </div>
                    </div>

                    <ul className="flex flex-col gap-3">
                        {events.map((e) => {
                            const pct = e.total_tickets > 0
                                ? Math.round((e.sold_tickets / e.total_tickets) * 100)
                                : 0;
                            return (
                                <li key={e.id}>
                                    <Link
                                        href={`/organizer/${e.id}`}
                                        className="flex flex-col gap-3 rounded-lg border border-border bg-surface p-5 shadow-e1 transition-[border-color,box-shadow] duration-[220ms] hover:border-border-strong hover:shadow-e2 sm:flex-row sm:items-center"
                                    >
                                        <div className="min-w-0 flex-1">
                                            <div className="tnum t-label mb-1 text-accent">
                                                {new Date(e.start_time).toLocaleDateString('en-US', {
                                                    weekday: 'short', month: 'short', day: 'numeric',
                                                })}
                                            </div>
                                            <h2 className="truncate">{e.name}</h2>
                                            {e.location && (
                                                <p className="text-[13px] text-text-muted">{e.location}</p>
                                            )}
                                        </div>

                                        <div className="flex items-center gap-6 sm:justify-end">
                                            <div className="text-right">
                                                <div className="tnum text-[15px] font-medium text-text">
                                                    {e.sold_tickets}<span className="text-text-muted">/{e.total_tickets}</span>
                                                </div>
                                                <div className="text-[12px] text-text-muted">sold</div>
                                            </div>
                                            <div className="text-right">
                                                <div className="tnum text-[15px] font-medium text-success">
                                                    ${e.revenue.toFixed(2)}
                                                </div>
                                                <div className="text-[12px] text-text-muted">revenue</div>
                                            </div>
                                            {/* A bar rather than only a number: proportion is the
                                                thing being read, and a bar shows it at a glance. */}
                                            <div className="hidden w-24 sm:block">
                                                <div className="h-1.5 overflow-hidden rounded-full bg-surface-sunken">
                                                    <div
                                                        className="h-full rounded-full bg-accent transition-[width] duration-[220ms]"
                                                        style={{ width: `${pct}%` }}
                                                    />
                                                </div>
                                                <div className="tnum mt-1 text-right text-[12px] text-text-muted">{pct}%</div>
                                            </div>
                                        </div>
                                    </Link>
                                </li>
                            );
                        })}
                    </ul>
                </>
            )}

            {status === 'ready' && events.length === 0 && (
                <div className="rounded-lg border border-dashed border-border-strong bg-surface-sunken px-6 py-12 text-center">
                    <h2 className="mb-1.5">No events yet</h2>
                    <p className="mx-auto mb-5 max-w-sm text-[15px] text-text-secondary">
                        Publish your first event, then add tickets to it.
                    </p>
                    <Link href="/organizer/new" className="inline-flex h-11 items-center rounded-md bg-accent px-5 text-[15px] font-medium text-on-accent transition-[background-color,transform] duration-[120ms] hover:bg-accent-hover active:scale-[.98]">
                        New event
                    </Link>
                </div>
            )}
        </div>
    );
}
