import React from 'react';
import {connect} from 'cerebral/react';
import {state} from 'cerebral/tags';

import moment from 'moment';
moment.locale('en-AU'); //FIXME: get the window locale


export default connect({
    },
    class CallbackSessions extends React.Component {
        constructor() {
            super();
        }
        render() {
            return (
                <div className="sessionlist">
                    <h1>Reverse Proxy Endpoints</h1>
                    <table className="table">
                        <thead>
                            <tr>
                                <th>Callback ID</th>
                                <th>Remote IP</th>
                                <th>Connected Clients</th>
                                <th>Establishment Time</th>
                            </tr>
                        </thead>
                        <tbody>
                        </tbody>
                    </table>
                </div>
            )
        }
    }
);
