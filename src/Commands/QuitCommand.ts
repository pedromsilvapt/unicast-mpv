import { Commands } from './Commands';
import { UnicastMpv } from '../UnicastMpv';
import { tuple } from '../Schema';

export class QuitCommand extends Commands {
    constructor ( server : UnicastMpv ) {
        super( server );

        this.register( 'quit', tuple( [] ), this.quit.bind( this ) );
    }

    public async quit () {
        if ( this.server.player.mpv.isRunning() ) {
            await this.server.player.mpv.quit();
        }
    }
}