'use client';

import { useRef, useState } from 'react';
import { Button } from '@/components/ui/Button';

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL ?? 'http://localhost:8000';

export interface Ticket {
    id: string;
    event_id: string;
    price: number;
    status: string;
    purchased_at: string;
    qr_code?: string | null;
}

export interface EventSummary {
    name: string;
    location: string;
    start_time: string;
}

type Cancel =
    | { kind: 'idle' }
    | { kind: 'working' }
    | { kind: 'done'; refunded: boolean }
    | { kind: 'error'; message: string };

export function TicketCard({
    ticket, event, highlighted, onCancelled,
}: {
    ticket: Ticket;
    event?: EventSummary;
    highlighted: boolean;
    onCancelled: (id: string) => void;
}) {
    const dialogRef = useRef<HTMLDialogElement>(null);
    const idempotencyKey = useRef<string | null>(null);
    const [state, setState] = useState<Cancel>({ kind: 'idle' });

    const cancel = async () => {
        if (state.kind === 'working') return;
        setState({ kind: 'working' });

        // Same key across retries, so a resubmitted cancellation replays rather
        // than attempting a second refund.
        if (idempotencyKey.current === null) {
            idempotencyKey.current = globalThis.crypto.randomUUID();
        }

        try {
            const res = await fetch(`${GATEWAY}/tickets/cancel`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    Authorization: `Bearer ${localStorage.getItem('token')}`,
                    'Idempotency-Key': idempotencyKey.current,
                },
                body: JSON.stringify({ ticket_id: ticket.id, reason: 'cancelled from My tickets' }),
            });

            if (!res.ok) {
                setState({
                    kind: 'error',
                    message: res.status === 409
                        ? 'This ticket has already been cancelled.'
                        : 'Could not cancel the ticket. Please try again.',
                });
                return;
            }

            const data = await res.json();
            dialogRef.current?.close();
            // The seat is already back in the pool even when the refund did not
            // go through, so this is reported as a distinct outcome rather than
            // a failure.
            setState({ kind: 'done', refunded: data.refund_status === 'refunded' });
            onCancelled(ticket.id);
        } catch {
            setState({ kind: 'error', message: 'Lost connection. Reload to check whether it went through.' });
        }
    };

    const start = event ? new Date(event.start_time) : null;

    return (
        <li
            className={[
                'flex flex-col gap-4 rounded-lg border bg-surface p-5 transition-[border-color] duration-[220ms] sm:flex-row',
                highlighted ? 'border-accent shadow-e2' : 'border-border shadow-e1',
            ].join(' ')}
        >
            {ticket.qr_code ? (
                /* The QR is a base64 PNG the API already produced, so it is
                   rendered directly rather than generated again in the browser.
                   eslint-disable-next-line @next/next/no-img-element -- next/image
                   cannot optimise a data URI, and this one is a small inline PNG
                   that is already in the response. */
                // eslint-disable-next-line @next/next/no-img-element
                <img
                    src={`data:image/png;base64,${ticket.qr_code}`}
                    alt={`Admission QR code for ticket ${ticket.id.slice(0, 8)}`}
                    className="h-28 w-28 flex-none rounded-md bg-white p-1.5"
                />
            ) : (
                <div className="flex h-28 w-28 flex-none items-center justify-center rounded-md bg-surface-sunken text-[13px] text-text-muted">
                    No QR
                </div>
            )}

            <div className="flex min-w-0 flex-1 flex-col gap-2">
                <div className="flex flex-wrap items-center gap-2">
                    <span className="t-label text-success">Confirmed</span>
                    {highlighted && (
                        <span className="rounded bg-accent-subtle px-2 py-0.5 text-[11px] font-semibold text-accent">
                            Just booked
                        </span>
                    )}
                </div>

                <h2 className="truncate">{event?.name ?? 'Event'}</h2>

                <div className="flex flex-col gap-0.5 text-[13px] text-text-muted">
                    {start && (
                        <span className="tnum">
                            {start.toLocaleDateString('en-US', { weekday: 'short', month: 'short', day: 'numeric' })}
                            {' · '}
                            {start.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}
                        </span>
                    )}
                    {event?.location && <span>{event.location}</span>}
                    <span className="tnum">${ticket.price.toFixed(2)}</span>
                </div>

                {state.kind === 'done' && (
                    <p className={`text-[13px] font-medium ${state.refunded ? 'text-success' : 'text-warning'}`}>
                        {state.refunded
                            ? 'Cancelled and refunded.'
                            : 'Cancelled. The refund is still processing — you will not be charged.'}
                    </p>
                )}

                {state.kind === 'error' && (
                    <p role="alert" className="text-[13px] font-medium text-danger">{state.message}</p>
                )}

                {state.kind !== 'done' && (
                    <div className="mt-auto pt-1">
                        <Button variant="secondary" onClick={() => dialogRef.current?.showModal()}>
                            Cancel ticket
                        </Button>
                    </div>
                )}
            </div>

            {/* Native dialog: it gets focus trapping, Escape to close and inert
                background from the platform rather than from hand-written code. */}
            <dialog
                ref={dialogRef}
                className="m-auto w-[min(26rem,calc(100vw-2rem))] rounded-lg border border-border bg-surface p-6 text-text backdrop:bg-black/50 backdrop:backdrop-blur-sm"
            >
                <h3 className="mb-2">Cancel this ticket?</h3>
                <p className="mb-1 text-[15px] text-text-secondary">
                    The seat returns to the pool immediately and someone else can book it.
                </p>
                <p className="mb-5 text-[15px] text-text-secondary">
                    ${ticket.price.toFixed(2)} will be refunded to your card.
                </p>

                {state.kind === 'error' && (
                    <p role="alert" className="mb-4 text-[13px] font-medium text-danger">{state.message}</p>
                )}

                <div className="flex gap-3">
                    <Button
                        variant="secondary"
                        onClick={() => dialogRef.current?.close()}
                        disabled={state.kind === 'working'}
                        className="flex-1"
                    >
                        Keep it
                    </Button>
                    <Button
                        variant="danger"
                        onClick={cancel}
                        loading={state.kind === 'working'}
                        className="flex-1"
                    >
                        {state.kind === 'working' ? 'Cancelling…' : 'Cancel ticket'}
                    </Button>
                </div>
            </dialog>
        </li>
    );
}
