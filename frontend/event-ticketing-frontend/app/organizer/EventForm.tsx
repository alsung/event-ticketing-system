'use client';

import { useState } from 'react';
import { Field } from '@/components/ui/Field';
import { Button } from '@/components/ui/Button';
import { Alert } from '@/components/ui/Alert';

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL ?? 'http://localhost:8000';

export interface EventDraft {
    name: string;
    description: string;
    location: string;
    start_time: string;
    end_time: string;
}

/** Shared by the create and edit pages, so the two cannot drift apart in
 *  validation or field order. */
export function EventForm({
    initial, submitLabel, eventId, onDone,
}: {
    initial: EventDraft;
    submitLabel: string;
    /** Present means edit (PUT to that event); absent means create. */
    eventId?: string;
    onDone: (id?: string) => void;
}) {
    const [draft, setDraft] = useState<EventDraft>(initial);
    const [error, setError] = useState('');
    const [submitting, setSubmitting] = useState(false);

    const set = (k: keyof EventDraft) => (v: string) =>
        setDraft((d) => ({ ...d, [k]: v }));

    const submit = async (e: React.FormEvent) => {
        e.preventDefault();
        if (submitting) return;
        setError('');

        if (!draft.name.trim()) { setError('Give the event a name.'); return; }
        if (new Date(draft.end_time) <= new Date(draft.start_time)) {
            setError('The event has to end after it starts.');
            return;
        }

        setSubmitting(true);
        try {
            const res = await fetch(
                eventId ? `${GATEWAY}/events/${eventId}` : `${GATEWAY}/events/create`,
                {
                    method: eventId ? 'PUT' : 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        Authorization: `Bearer ${localStorage.getItem('token')}`,
                    },
                    body: JSON.stringify({
                        ...draft,
                        // The API wants RFC3339; the datetime-local input gives
                        // "2026-12-01T20:00" with no zone.
                        start_time: new Date(draft.start_time).toISOString(),
                        end_time: new Date(draft.end_time).toISOString(),
                    }),
                },
            );

            if (!res.ok) {
                setError(
                    res.status === 403 ? 'Your account cannot publish events.'
                    : res.status === 404 ? 'That event no longer exists, or is not yours to edit.'
                    : res.status === 400 ? 'Check the dates and the name, then try again.'
                    : 'Could not save the event. Please try again.',
                );
                return;
            }

            const data = eventId ? null : await res.json();
            onDone(data?.id);
        } catch {
            setError('Could not reach the server. Check your connection and try again.');
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
            {error && <Alert>{error}</Alert>}

            <Field label="Event name" value={draft.name} onChange={set('name')} required disabled={submitting} />
            <Field label="Description" value={draft.description} onChange={set('description')}
                   hint="What the night is, who is playing." disabled={submitting} />
            <Field label="Location" value={draft.location} onChange={set('location')}
                   hint="City and venue." disabled={submitting} />

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <Field label="Starts" type="datetime-local" value={draft.start_time}
                       onChange={set('start_time')} required disabled={submitting} />
                <Field label="Ends" type="datetime-local" value={draft.end_time}
                       onChange={set('end_time')} required disabled={submitting} />
            </div>

            <Button type="submit" loading={submitting} fullWidth>
                {submitting ? 'Saving…' : submitLabel}
            </Button>
        </form>
    );
}

/** Converts an RFC3339 timestamp into the value a datetime-local input wants. */
export function toLocalInput(iso: string): string {
    const d = new Date(iso);
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}
