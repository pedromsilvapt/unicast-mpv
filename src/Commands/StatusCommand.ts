import { Commands } from './Commands';
import { UnicastMpv } from '../UnicastMpv';
import { tuple } from '../Schema';
import { changeObjectCase } from '../Utils/ChangeObjectCase';

export class StatusCommand extends Commands {
    constructor ( server : UnicastMpv ) {
        super( server );

        this.register( 'status', tuple( [] ), this.status.bind( this ) );
        
        this.server.registerPostHook( 'quit', () => this.server.player.status.stop() );
        this.server.registerPostHook( 'stop', () => this.server.player.status.stop() );
        this.server.registerPreHook( 'play', () => this.server.player.status.play() );

        this.server.player.mpv.on( 'status', status => this.server.player.status.update( changeObjectCase( status ) as any ) );
    }

    public async status () {
        return await this.server.player.status.get();
    }
}