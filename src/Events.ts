import { UnicastMpv } from './UnicastMpv';

export class Events {
    public readonly server : UnicastMpv;
    
    constructor ( server : UnicastMpv ) {
        this.server = server;

        for ( let event of [ 'started', 'stopped', 'paused', 'resumed', 'seek', 'status', 'quit', 'crashed' ] ) {
            this.server.event( event );
        }

        const mpv = this.server.player.mpv;

        mpv.on( 'started', () => this.server.emit( 'started' ) );
        mpv.on( 'stopped', () => this.server.emit( 'stopped' ) );
        mpv.on( 'paused', () => this.server.emit( 'paused' ) );
        mpv.on( 'resumed', () => this.server.emit( 'resumed' ) );
        mpv.on( 'seek', data => this.server.emit( 'seek', data ) );
        mpv.on( 'statuschange', data => this.server.emit( 'status', data ) );
        mpv.on( 'quit', data => this.server.emit( 'quit', data ) );
        mpv.on( 'crashed', data => this.server.emit( 'crashed', data ) );
    }
}