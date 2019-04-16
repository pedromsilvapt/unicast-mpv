import { UnicastMpv } from './UnicastMpv';

export class Events {
    public readonly server : UnicastMpv;
    
    constructor ( server : UnicastMpv ) {
        this.server = server;

        const connection = this.server.connection;

        for ( let event of [ 'started', 'stopped', 'paused', 'resumed', 'seek', 'status' ] ) {
            connection.event( event );
        }

        const mpv = this.server.player.mpv;

        mpv.on( 'started', () => connection.emit( 'started' ) );
        mpv.on( 'stopped', () => connection.emit( 'stopped' ) );
        mpv.on( 'paused', () => connection.emit( 'paused' ) );
        mpv.on( 'resumed', () => connection.emit( 'resumed' ) );
        mpv.on( 'seek', data => connection.emit( 'seek', data ) );
        mpv.on( 'statuschange', data => connection.emit( 'status', data ) );
    }
}