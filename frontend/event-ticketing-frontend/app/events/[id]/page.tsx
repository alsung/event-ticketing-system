import Link from 'next/link';
import { notFound } from 'next/navigation';
import { Availability } from './Availability';

/** Server Component. The event itself is public, so it renders on the server;
 *  availability needs the caller's token, so it is fetched by a client
 *  component below. Splitting it that way keeps the page server-rendered
 *  without leaking a token into the server fetch. */

const GATEWAY = process.env.GATEWAY_URL ?? 'http://localhost:8000';

interface EventDetail {
    id: string;
    name: string;
    description: string;
    location: string;
    start_time: string;
    end_time: string;
}

async function getEvent(id: string): Promise<EventDetail | null> {
    const res = await fetch(`${GATEWAY}/events/${id}`, { cache: 'no-store' });
    if (res.status === 404 || res.status === 400) return null;
    if (!res.ok) throw new Error(`event request failed: ${res.status}`);
    return res.json();
}

export default async function EventDetailPage({
    params,
}: {
    params: Promise<{ id: string }>;
}) {
    // params is a promise in this version of Next; awaiting it is required.
    const { id } = await params;
    const event = await getEvent(id);

    // A missing event is not an error condition — notFound renders the 404
    // boundary rather than the error boundary, which is the honest distinction.
    if (!event) notFound();

    const start = new Date(event.start_time);
    const end = new Date(event.end_time);

    return (
        <div className="flex flex-col gap-8">
            <nav aria-label="Breadcrumb">
                <Link
                    href="/events"
                    className="inline-flex items-center gap-1.5 text-sm text-text-secondary transition-colors duration-[120ms] hover:text-accent"
                >
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                        <path d="M15 18l-6-6 6-6" stroke="currentColor" strokeWidth="2"
                            strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                    All events
                </Link>
            </nav>

            <header className="flex flex-col gap-3">
                <span className="t-label text-accent">
                    {start.toLocaleDateString('en-US', {
                        weekday: 'long', month: 'long', day: 'numeric', year: 'numeric',
                    })}
                </span>
                <h1>{event.name}</h1>
                {event.description && (
                    <p className="max-w-2xl text-lg leading-relaxed text-text-secondary">
                        {event.description}
                    </p>
                )}
            </header>

            <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_320px]">
                <dl className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-5">
                    <div className="flex flex-col gap-0.5">
                        <dt className="t-label text-text-muted">When</dt>
                        <dd className="tnum text-[15px] text-text">
                            {start.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}
                            {' – '}
                            {end.toLocaleTimeString('en-US', { hour: 'numeric', minute: '2-digit' })}
                        </dd>
                    </div>
                    {event.location && (
                        <div className="flex flex-col gap-0.5">
                            <dt className="t-label text-text-muted">Where</dt>
                            <dd className="text-[15px] text-text">{event.location}</dd>
                        </div>
                    )}
                </dl>

                <Availability eventId={event.id} eventName={event.name} />
            </div>
        </div>
    );
}
