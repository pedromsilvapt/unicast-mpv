import { Commands } from './Commands';
import { UnicastMpv } from '../UnicastMpv';
import { string, tuple, optional, any } from '../Schema';
import { LoadFlags, LoadOptions } from '../Player';

export class PlayCommand extends Commands {
    constructor ( server : UnicastMpv ) {
        super( server );

        this.register( 'play', tuple( [ string(), optional( string() ), any() ] ), this.play.bind( this ) );
    }

    public async play ( file : string, subtitles : string = null, options : LoadOptions = {} ) {
        if ( this.server.player.config.get( 'restartOnPlay' ) == true ) {
            await this.server.player.mpv.quit();
        }
        
        if ( !this.server.player.mpv.isRunning() ) {
            await this.server.player.start();
        }

        await this.server.player.load( file, LoadFlags.Replace, options );
        
        if ( subtitles ) {
            await this.server.player.mpv.addSubtitles( subtitles );
        }
    }
}