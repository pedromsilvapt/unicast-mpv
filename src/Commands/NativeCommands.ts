import { string, optional, number, any, object, boolean } from '../Schema';
import { Commands } from './Commands';
import { UnicastMpv } from '../UnicastMpv';

export class NativeCommands extends Commands {
    constructor ( server : UnicastMpv ) {
        super( server );

        this.registerNative( 'load', [ string(), optional( string() ) ] );
        this.registerNative( 'stop' );
        this.registerNative( 'pause' );
        this.registerNative( 'resume' );
        this.registerNative( 'seek', [ number() ] );
        this.registerNative( 'goToPosition', [ number() ] );
        this.registerNative( 'mute', [ boolean() ] );
        this.registerNative( 'volume', [ number() ] );

        this.registerNative( 'setProperty', [ string(), any() ] );
        this.registerNative( 'setMultipleProperties', [ object() ] );
        this.registerNative( 'getProperty', [ string() ] );
        this.registerNative( 'addProperty', [ string(), number() ] );
        this.registerNative( 'multiplyProperty', [ string(), number() ] );
        this.registerNative( 'cycleProperty', [ string() ] );

        this.registerNative( 'subtitleScale', [ number() ] );
        this.registerNative( 'adjustSubtitleTiming', [ number() ] );
        this.registerNative( 'hideSubtitles' );
        this.registerNative( 'showSubtitles' );

        this.register( 'showProgress', [], () => this.server.player.mpv.command( 'show-progress', [] ) );
    }
}