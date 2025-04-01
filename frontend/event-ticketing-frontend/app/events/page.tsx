'use client';

import { useState, useEffect } from "react";

interface Event {
    id: string;
    name: string;
    description: string;
    location: string;
    start_time: string;
    end_time: string;
    organizer_id: string;
    created_at: string;
}

export default function EventsPage() {
    const [events, setEvents] = useState<Event[]>([]);
    const [error, setError] = useState('');
    
    useEffect(() => {
        const fetchEvents = async () => {
            const token = localStorage.getItem('token');

            try {
                const res = await fetch('http://localhost:8000/events', {
                    method: 'GET',
                    headers: {
                        'Authorization': `Bearer ${token}`,
                        'Content-Type': 'application/json',
                    },
                });

                if (!res.ok) {
                    throw new Error('Failed to fetch events');
                }

                const data = await res.json();
                setEvents(data);
            } catch (err) {
                setError('Could not load events. Please try again.');
                console.error(err);
            }
        };

        fetchEvents();
    }, []);
    
    return (
        <div className="min-h-screen p-8 bg-gray-50">
            <h1 className="text-2xl font-bold mb-6 text-center">Upcoming Events</h1>

            {error && (
                <p className="text-red-500 text-center mb-4">{error}</p>
            )}

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                {events.map((event) => (
                    <div key={event.id} className="bg-white p-6 rounded shadow-md">
                        <h2 className="text-xl font-semibold mb-2">{event.name}</h2>
                        <p className="text-sm text-gray-600 mb-2">{event.location}</p>
                        <p className="text-gray-700 mb-4">{event.description}</p>
                        <p className="text-sm text-gray-500">Starts: {new Date(event.start_time).toLocaleString()}</p>
                        <p className="text-sm text-gray-500">Ends: {new Date(event.end_time).toLocaleString()}</p>
                    </div>
                ))}
            </div>
        </div>
    );
}