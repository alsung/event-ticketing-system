'use client';

import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { EventForm, toLocalInput } from '../EventForm';

// Default to a sensible evening a week out, so the form opens with valid dates
// rather than empty pickers. Computed at module scope, not during render:
// Date.now is impure, and a value that changed between renders would reset the
// form under the person filling it in.
const defaultStart = (() => {
    const d = new Date(Date.now() + 7 * 864e5);
    d.setHours(19, 0, 0, 0);
    return d;
})();
const defaultEnd = new Date(defaultStart.getTime() + 3 * 36e5);

export default function NewEventPage() {
    const router = useRouter();

    return (
        <div className="mx-auto flex max-w-lg flex-col gap-6">
            <nav aria-label="Breadcrumb">
                <Link href="/organizer" className="inline-flex items-center gap-1.5 text-sm text-text-secondary transition-colors duration-[120ms] hover:text-accent">
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                        <path d="M15 18l-6-6 6-6" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                    Your events
                </Link>
            </nav>

            <div className="flex flex-col gap-1.5">
                <h1>New event</h1>
                <p className="text-[15px] text-text-secondary">
                    Publish the event first, then add tickets to it.
                </p>
            </div>

            <div className="rounded-lg border border-border bg-surface p-6 shadow-e2">
                <EventForm
                    submitLabel="Publish event"
                    initial={{
                        name: '', description: '', location: '',
                        start_time: toLocalInput(defaultStart.toISOString()),
                        end_time: toLocalInput(defaultEnd.toISOString()),
                    }}
                    // Straight to the manage page: an event with no tickets
                    // cannot be bought, so adding inventory is the next step.
                    onDone={(id) => router.push(id ? `/organizer/${id}` : '/organizer')}
                />
            </div>
        </div>
    );
}
