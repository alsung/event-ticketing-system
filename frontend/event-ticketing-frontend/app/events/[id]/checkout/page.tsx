import Link from 'next/link';
import { notFound } from 'next/navigation';
import { CheckoutForm } from './CheckoutForm';

const GATEWAY = process.env.GATEWAY_URL ?? 'http://localhost:8000';

interface EventDetail {
    id: string;
    name: string;
    start_time: string;
    location: string;
}

async function getEvent(id: string): Promise<EventDetail | null> {
    const res = await fetch(`${GATEWAY}/events/${id}`, { cache: 'no-store' });
    if (res.status === 404 || res.status === 400) return null;
    if (!res.ok) throw new Error(`event request failed: ${res.status}`);
    return res.json();
}

export default async function CheckoutPage({
    params,
}: {
    params: Promise<{ id: string }>;
}) {
    const { id } = await params;
    const event = await getEvent(id);
    if (!event) notFound();

    const start = new Date(event.start_time);

    return (
        <div className="mx-auto flex max-w-lg flex-col gap-6">
            <nav aria-label="Breadcrumb">
                <Link
                    href={`/events/${event.id}`}
                    className="inline-flex items-center gap-1.5 text-sm text-text-secondary transition-colors duration-[120ms] hover:text-accent"
                >
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                        <path d="M15 18l-6-6 6-6" stroke="currentColor" strokeWidth="2"
                            strokeLinecap="round" strokeLinejoin="round" />
                    </svg>
                    Back to event
                </Link>
            </nav>

            <header className="flex flex-col gap-1.5">
                <h1>Checkout</h1>
                <p className="text-[15px] text-text-secondary">
                    {event.name} · {start.toLocaleDateString('en-US', {
                        weekday: 'short', month: 'short', day: 'numeric',
                    })}
                    {event.location && ` · ${event.location}`}
                </p>
            </header>

            <CheckoutForm eventId={event.id} eventName={event.name} />
        </div>
    );
}
