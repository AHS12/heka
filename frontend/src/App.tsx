import {useEffect, useState} from 'react';
import './App.css';
import {AppInfo} from '../wailsjs/go/app/App';

interface Info {
    name: string;
    version: string;
    daemon: string;
}

function App() {
    const [info, setInfo] = useState<Info | null>(null);

    useEffect(() => {
        AppInfo()
            .then(setInfo)
            .catch(() => setInfo(null));
    }, []);

    return (
        <div className="shell">
            <header className="titlebar">
                <span className="app-name">{info?.name ?? 'Heka'}</span>
                <span className="version">v{info?.version ?? '…'}</span>
            </header>
            <main className="content">
                <div className={`pill ${info?.daemon === 'running' ? 'ok' : 'warn'}`}>
                    Daemon: {info?.daemon ?? 'unknown'}
                </div>
                <p className="hint">Frontend bridge works. Real UI arrives in SPEC-11.</p>
            </main>
        </div>
    );
}

export default App;