'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';

export default function LoginPage() {
    const router = useRouter();

    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');

    const handleLogin = async (e: React.FormEvent) => {
        e.preventDefault();
        setError('');

        try {
            const res = await fetch('http://localhost:8000/users/login', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ email, password }),
            });

            const data = await res.json();

            if (!res.ok) {
                setError(data.error || 'Login failed');
                return;
            }

            // Save token to localStorage
            localStorage.setItem('token', data.token);
            router.push('/events'); // Redirect after login (you can change the path at any time)
        } catch (err) {
            setError('Something went wrong. Please try again.');
        }
    };

    return (
        <div className="min-h-screen flex items-center justify-center bg-gray-100">
            <form onSubmit={handleLogin} className="bg-white p-8 rounded shadow-md w-full max-w-md">
                <h1 className="text-2xl font-semibold mb-6 text-center text-gray-800">Login</h1>

                {error && <p className="text-red-500 text-sm mb-4">{error}</p>}

                <label className="block mb-2 text-sm font-semibold text-gray-800">Email</label>
                <input
                    type="email"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    required
                    className="w-full p-2 mb-4 border border-gray-400 rounded text-gray-900"
                />

                <label className="block mb-2 text-sm font-semibold text-gray-800">Password</label>
                <input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    required
                    className="w-full p-2 mb-6 border border-gray-400 rounded text-gray-900"
                />

                <button
                    type="submit"
                    className="w-full bg-blue-600 text-white py-2 rounded hover:bg-blue-700 transition"
                >
                Login
                </button>
            </form>
        </div>
    );
}