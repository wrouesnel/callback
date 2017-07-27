import React from 'react';
import {connect} from 'cerebral/react';

import controller from 'controller';
import CallbackSessions from 'components/CallbackSessions';
import ClientSessions from 'components/ClientSessions';

export default connect({},
    class App extends React.Component {
        componentDidMount() {
            controller.getSignal('appMounted')();
        }
        render() {
            return (
                <div className="container-fluid">
                    <CallbackSessions/>
                    <ClientSessions/>
                </div>
            );
        }
    }
);
